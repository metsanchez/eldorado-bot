package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"eldorado-bot/internal/config"
	"eldorado-bot/internal/eldorado"
	"eldorado-bot/internal/logger"
	"eldorado-bot/internal/storage"
	"eldorado-bot/internal/telegram"
)

const (
	authAlertThreshold    = 3
	msgAlertThreshold     = 3
	criticalAlertCooldown = time.Hour
)

type Runner struct {
	log     *logger.Logger
	cfg     *config.Config
	eld     *eldorado.Client
	tg      *telegram.Client
	storage *storage.JSONStorage
	chatMu  sync.Mutex // serialize buyer messages — only one Chrome at a time
	msgWg   sync.WaitGroup // for graceful shutdown: wait for in-flight messages

	// Critical error tracking
	authFailMu    sync.Mutex
	authFailCount int
	lastAuthAlert time.Time

	msgFailMu    sync.Mutex
	msgFailCount int
	lastMsgAlert time.Time
}

func NewRunner(
	log *logger.Logger,
	cfg *config.Config,
	eldClient *eldorado.Client,
	tgClient *telegram.Client,
	st *storage.JSONStorage,
) *Runner {
	return &Runner{
		log:     log,
		cfg:     cfg,
		eld:     eldClient,
		tg:      tgClient,
		storage: st,
	}
}

func (r *Runner) alertCritical(ctx context.Context, title, detail string) {
	r.tg.SendMessage(ctx, fmt.Sprintf("⚠️ <b>%s</b>\n\n%s", title, detail))
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, eldorado.ErrAuthExpired) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "re-login failed") || strings.Contains(s, "401") || strings.Contains(s, "403")
}

func (r *Runner) trackAuthError(ctx context.Context, err error) {
	if !isAuthError(err) {
		return
	}
	r.authFailMu.Lock()
	defer r.authFailMu.Unlock()
	r.authFailCount++
	if r.authFailCount >= authAlertThreshold && time.Since(r.lastAuthAlert) > criticalAlertCooldown {
		r.alertCritical(ctx, "Eldorado Auth Hatası",
			fmt.Sprintf("API/Login %d kez üst üste başarısız.\n\nSon hata: %v", r.authFailCount, err))
		r.lastAuthAlert = time.Now()
	}
}

func (r *Runner) resetAuthFailCount() {
	r.authFailMu.Lock()
	defer r.authFailMu.Unlock()
	r.authFailCount = 0
}

func (r *Runner) trackMsgError(ctx context.Context, requestID string, err error) {
	r.msgFailMu.Lock()
	defer r.msgFailMu.Unlock()
	r.msgFailCount++
	if r.msgFailCount >= msgAlertThreshold && time.Since(r.lastMsgAlert) > criticalAlertCooldown {
		r.alertCritical(ctx, "Alıcı Mesaj Hatası",
			fmt.Sprintf("Mesaj gönderme %d kez üst üste başarısız.\n\nSon request: %s\nSon hata: %v",
				r.msgFailCount, requestID, err))
		r.lastMsgAlert = time.Now()
	}
}

func (r *Runner) resetMsgFailCount() {
	r.msgFailMu.Lock()
	defer r.msgFailMu.Unlock()
	r.msgFailCount = 0
}

func (r *Runner) Start(ctx context.Context) error {
	if r.cfg.BuyerAutoMessage != "" {
		r.log.Infof("buyer auto-message enabled (%d chars)", len(r.cfg.BuyerAutoMessage))
	}

	if err := r.eld.Login(ctx); err != nil {
		r.alertCritical(ctx, "Eldorado Login Hatası", fmt.Sprintf("Bot başlatılamadı: %v", err))
		return err
	}

	errCh := make(chan error, 3)

	go func() {
		if err := r.runBoostingRequestsLoop(ctx); err != nil {
			errCh <- err
		}
	}()

	go func() {
		if err := r.runOfferStatusLoop(ctx); err != nil {
			errCh <- err
		}
	}()

	go r.runStatsCommandLoop(ctx)

	select {
	case <-ctx.Done():
		r.log.Infof("shutdown requested, waiting for in-flight messages (max 90s)...")
		done := make(chan struct{})
		go func() {
			r.msgWg.Wait()
			close(done)
		}()
		select {
		case <-done:
			r.log.Infof("all messages sent, shutdown complete")
		case <-time.After(90 * time.Second):
			r.log.Infof("shutdown timeout, exiting")
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func (r *Runner) runStatsCommandLoop(ctx context.Context) {
	if r.cfg.TelegramBotToken == "" || r.cfg.TelegramChatID == 0 {
		return
	}
	r.log.Infof("stats command listener started (/stats)")
	var offset int
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := r.tg.GetUpdates(ctx, offset)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Chat == nil || u.Message.Text == "" {
				continue
			}
			text := strings.TrimSpace(u.Message.Text)
			if text != "/stats" {
				continue
			}
			st := r.storage.GetStats()
			winRate := 0.0
			if st.OffersWon+st.OffersLost > 0 {
				winRate = float64(st.OffersWon) / float64(st.OffersWon+st.OffersLost) * 100
			}
			msg := fmt.Sprintf(
				"<b>📊 Bot İstatistikleri</b>\n\n"+
					"Toplam teklif: <b>%d</b>\n"+
					"Kazanılan: <b>%d</b>\n"+
					"Kaybedilen: <b>%d</b>\n"+
					"Kazanma oranı: <b>%.1f%%</b>\n"+
					"Gönderilen mesaj: <b>%d</b>\n\n"+
					"Son güncelleme: %s",
				st.OffersCreated, st.OffersWon, st.OffersLost, winRate, st.MessagesSent,
				st.LastUpdated.Format("02.01.2006 15:04"))

			_ = r.tg.SendMessageToChat(ctx, u.Message.Chat.ID, msg)
		}
	}
}

func (r *Runner) runBoostingRequestsLoop(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollIntervalOpenOrders)
	defer ticker.Stop()

	r.log.Infof("starting boosting requests polling loop (interval=%s)", r.cfg.PollIntervalOpenOrders)

	for {
		if err := retryWithBackoff(ctx, r.log, 3, time.Second, r.handleBoostingRequestsOnce); err != nil {
			r.log.Errorf("boosting requests loop error after retries: %v", err)
			r.trackAuthError(ctx, err)
		} else {
			r.resetAuthFailCount()
		}

		select {
		case <-ctx.Done():
			r.log.Infof("stopping boosting requests loop")
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) handleBoostingRequestsOnce(ctx context.Context) error {
	page, err := r.eld.ListReceivedBoostingRequests(ctx, eldorado.FilterActiveRequests, r.cfg.ValorantGameID)
	if err != nil {
		return err
	}

	r.log.Infof("fetched %d boosting requests (filter=%s, gameId=%s)", len(page.Results), eldorado.FilterActiveRequests, r.cfg.ValorantGameID)

	for _, item := range page.Results {
		if r.storage.IsOrderSeen(item.ID) {
			continue
		}

		if err := r.storage.MarkOrderSeen(item.ID); err != nil {
			r.log.Errorf("mark request seen failed (id=%s): %v", item.ID, err)
		}

		if item.IsBuyerMuted {
			r.log.Infof("skipping muted buyer request %s", item.ID)
			continue
		}

		// Skip unsupported categories
		cat := item.BoostingCategoryID
		if cat != eldorado.CategoryRankBoost && cat != eldorado.CategoryNetWins {
			r.log.Infof("skipping request %s (category=%s/%s)", item.ID, cat, item.BoostingCategoryTitle)
			continue
		}

		// Fetch full details for rank info
		detail, err := r.eld.GetBoostingRequestDetails(ctx, item.ID)
		if err != nil {
			r.log.Errorf("fetch request detail failed (id=%s): %v", item.ID, err)
			continue
		}

		var result PriceResult
		switch cat {
		case eldorado.CategoryRankBoost:
			result = CalculateRankBoostPrice(detail)
		case eldorado.CategoryNetWins:
			result = CalculateNetWinPrice(detail)
		}

		if result.Skip {
			r.log.Infof("skipping request %s: %s (buyer=%s, category=%s)",
				item.ID, result.SkipReason, item.BuyerUsername, item.BoostingCategoryTitle)
			continue
		}

		// Floor: RR discount can push price very low — enforce minimum
		if r.cfg.MinOfferPrice > 0 && result.Price < r.cfg.MinOfferPrice {
			r.log.Infof("price floor applied: $%.2f -> $%.2f (request %s)", result.Price, r.cfg.MinOfferPrice, item.ID)
			result.Price = r.cfg.MinOfferPrice
		}

		r.log.Infof("creating offer for request %s (buyer=%s, category=%s, price=$%.2f, delivery=%s)",
			item.ID, item.BuyerUsername, item.BoostingCategoryTitle, result.Price, result.DeliveryTime)

		// Log rank details for visibility
		if cat == eldorado.CategoryRankBoost {
			r.log.Infof("  rank: %s -> %s", detail.GetDescValue("Current Rank"), detail.GetDescValue("Desired rank"))
		} else if cat == eldorado.CategoryNetWins {
			r.log.Infof("  rank: %s, games: %s", detail.GetDescValue("Current season rank"), detail.GetDescValue("Number of games"))
		}

		offerReq := eldorado.BoostingOfferPost{
			Details: eldorado.BoostingOfferDetails{
				BoostingRequestID:      item.ID,
				GuaranteedDeliveryTime: result.DeliveryTime,
				Pricing: eldorado.OfferPricing{
					Quantity:    1,
					MinQuantity: 1,
					PricePerUnit: eldorado.MoneyBase{
						Amount:   result.Price,
						Currency: "USD",
					},
				},
			},
		}

		offer, err := r.eld.CreateBoostingOffer(ctx, offerReq)
		if err != nil {
			r.log.Errorf("create offer failed (requestId=%s): %v", item.ID, err)
			continue
		}

		r.log.Infof("offer created (offerId=%s, requestId=%s)", offer.ID, item.ID)

		// Send Telegram notification about the offer
		msg := fmt.Sprintf(
			"<b>Teklif Verildi!</b>\n\n"+
				"Fiyat: <b>$%.2f</b>\n"+
				"Kategori: %s\n"+
				"Method: <b>%s</b>\n"+
				"Alici: %s\n"+
				"Teslimat: %s",
			result.Price, item.BoostingCategoryTitle, result.Method, item.BuyerUsername, result.DeliveryTime)
		if cat == eldorado.CategoryRankBoost {
			msg += fmt.Sprintf("\nRank: <b>%s</b> ➜ <b>%s</b>", detail.GetDescValue("Current Rank"), detail.GetDescValue("Desired rank"))
		} else if cat == eldorado.CategoryNetWins {
			msg += fmt.Sprintf("\nRank: <b>%s</b> | Oyun: <b>%s</b>", detail.GetDescValue("Current season rank"), detail.GetDescValue("Number of games"))
		}
		if cat == eldorado.CategoryRankBoost || cat == eldorado.CategoryNetWins {
			if rr := detail.GetDescValue("Current RR"); rr != "" {
				msg += fmt.Sprintf("\nRR: <b>%s</b>", rr)
			}
		}
		msg += fmt.Sprintf("\n\nID: <code>%s</code>", item.ID)
		if err := r.tg.SendMessage(ctx, msg); err != nil {
			r.log.Errorf("telegram send failed: %v", err)
		}

		detailRank := detail.GetDescValue("Current Rank")
		detailDesired := detail.GetDescValue("Desired rank")
		if detailRank == "" {
			detailRank = detail.GetDescValue("Current season rank")
		}
		if err := r.storage.TrackOrderWithDetails(item.ID, "OfferSubmitted", storage.StatusOfferPending,
			result.Price, detailRank, detailDesired, detail.GetDescValue("Current RR"), item.BoostingCategoryTitle); err != nil {
			r.log.Errorf("track order failed (requestId=%s): %v", item.ID, err)
		}
		r.storage.IncrementOffersCreated()

		// Create conversation and send auto-message to buyer
		r.msgWg.Add(1)
		go r.sendBuyerMessage(ctx, item.ID)
	}

	return nil
}

func (r *Runner) runOfferStatusLoop(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollIntervalOrderStatus)
	defer ticker.Stop()

	r.log.Infof("starting offer status polling loop (interval=%s)", r.cfg.PollIntervalOrderStatus)

	for {
		if err := retryWithBackoff(ctx, r.log, 3, time.Second, r.handleOfferStatusOnce); err != nil {
			r.log.Errorf("offer status loop error after retries: %v", err)
			r.trackAuthError(ctx, err)
		} else {
			r.resetAuthFailCount()
		}

		select {
		case <-ctx.Done():
			r.log.Infof("stopping offer status loop")
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) handleOfferStatusOnce(ctx context.Context) error {
	wonPage, err := r.eld.ListReceivedBoostingRequests(ctx, eldorado.FilterOfferWon, "")
	if err != nil {
		return err
	}

	r.log.Infof("checking offer status: %d won", len(wonPage.Results))

	for _, item := range wonPage.Results {
		tracked := r.storage.ListTrackedOrdersByStatus(storage.StatusOfferPending)
		for _, tr := range tracked {
			if tr.OrderID == item.ID {
				r.log.Infof("offer WON for request %s (buyer=%s)!", item.ID, item.BuyerUsername)
				catTitle := tr.CategoryTitle
				if catTitle == "" {
					catTitle = item.BoostingCategoryTitle
				}
				r.tg.NotifyOrderAssignedWithDetails(ctx, item.ID, item.BuyerUsername, catTitle, item.GameID,
					tr.OfferPrice, tr.CurrentRank, tr.DesiredRank, tr.CurrentRR)
				if err := r.storage.UpdateTrackedOrderStatus(item.ID, "OfferWon", storage.StatusAssigned); err != nil {
					r.log.Errorf("update tracked order failed (id=%s): %v", item.ID, err)
				}
				r.storage.IncrementOffersWon()
				break
			}
		}
	}

	lostPage, err := r.eld.ListReceivedBoostingRequests(ctx, eldorado.FilterOfferLost, "")
	if err != nil {
		return err
	}

	for _, item := range lostPage.Results {
		pending := r.storage.ListTrackedOrdersByStatus(storage.StatusOfferPending)
		for _, tr := range pending {
			if tr.OrderID == item.ID {
				r.log.Infof("offer LOST for request %s", item.ID)
				if err := r.storage.UpdateTrackedOrderStatus(item.ID, "OfferLost", storage.StatusClosed); err != nil {
					r.log.Errorf("update tracked order failed (id=%s): %v", item.ID, err)
				}
				r.storage.IncrementOffersLost()
				break
			}
		}
	}

	return nil
}

func (r *Runner) sendBuyerMessage(ctx context.Context, requestID string) {
	defer r.msgWg.Done()
	if r.cfg.BuyerAutoMessage == "" {
		r.log.Infof("buyer auto-message skipped (no message configured)")
		return
	}

	// Serialize: only one Chrome instance at a time — avoids VPS overload when multiple orders
	r.chatMu.Lock()
	defer r.chatMu.Unlock()

	r.log.Infof("sending buyer auto-message for request %s...", requestID[:min(len(requestID), 12)])

	conv, err := r.eld.CreateConversationForSeller(ctx, requestID)
	if err != nil {
		r.log.Errorf("create conversation failed (requestId=%s): %v", requestID, err)
		r.trackMsgError(ctx, requestID, err)
		return
	}
	r.log.Infof("conversation created (requestId=%s, talkJsId=%s)", requestID, conv.TalkJsConversationID)

	if err := eldorado.SendChatMessage(ctx, requestID, r.cfg.BuyerAutoMessage, r.cfg.BuyerAutoImage, r.log); err != nil {
		r.log.Errorf("send buyer message failed (requestId=%s): %v", requestID, err)
		r.trackMsgError(ctx, requestID, err)
		return
	}
	r.storage.IncrementMessagesSent()
	r.resetMsgFailCount()
	r.log.Infof("buyer auto-message sent successfully (requestId=%s)", requestID[:min(len(requestID), 12)])
}
