package logic

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	offerSummaryInterval  = 10 * time.Minute
)

type Runner struct {
	log     *logger.Logger
	cfg     *config.Config
	eld     *eldorado.Client
	tg      *telegram.Client
	storage *storage.JSONStorage
	chatMu  sync.Mutex     // serialize buyer messages — only one Chrome at a time
	msgWg   sync.WaitGroup // for graceful shutdown: wait for in-flight messages
	sumMu   sync.Mutex
	sumData offerSummaryData

	// Critical error tracking
	authFailMu    sync.Mutex
	authFailCount int
	lastAuthAlert time.Time

	msgFailMu    sync.Mutex
	msgFailCount int
	lastMsgAlert time.Time
}

type offerSummaryData struct {
	Count       int
	TotalAmount float64
	ByCategory  map[string]int
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
		sumData: offerSummaryData{
			ByCategory: make(map[string]int),
		},
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
	go r.runOfferSummaryLoop(ctx)
	go r.runBuyerReplyNotifyLoop(ctx)

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

func (r *Runner) recordOfferSummary(category string, amount float64) {
	r.sumMu.Lock()
	defer r.sumMu.Unlock()

	if r.sumData.ByCategory == nil {
		r.sumData.ByCategory = make(map[string]int)
	}

	r.sumData.Count++
	r.sumData.TotalAmount += amount
	if strings.TrimSpace(category) == "" {
		category = "Unknown"
	}
	r.sumData.ByCategory[category]++
}

func (r *Runner) snapshotAndResetOfferSummary() offerSummaryData {
	r.sumMu.Lock()
	defer r.sumMu.Unlock()

	snapshot := offerSummaryData{
		Count:       r.sumData.Count,
		TotalAmount: r.sumData.TotalAmount,
		ByCategory:  make(map[string]int, len(r.sumData.ByCategory)),
	}
	for k, v := range r.sumData.ByCategory {
		snapshot.ByCategory[k] = v
	}

	r.sumData.Count = 0
	r.sumData.TotalAmount = 0
	r.sumData.ByCategory = make(map[string]int)
	return snapshot
}

func (r *Runner) runOfferSummaryLoop(ctx context.Context) {
	if r.cfg.TelegramBotToken == "" || r.cfg.TelegramChatID == 0 {
		return
	}

	ticker := time.NewTicker(offerSummaryInterval)
	defer ticker.Stop()
	r.log.Infof("offer summary loop started (interval=%s)", offerSummaryInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			summary := r.snapshotAndResetOfferSummary()
			if summary.Count == 0 {
				continue
			}

			lines := make([]string, 0, len(summary.ByCategory))
			categories := make([]string, 0, len(summary.ByCategory))
			for category := range summary.ByCategory {
				categories = append(categories, category)
			}
			sort.Strings(categories)
			for _, category := range categories {
				lines = append(lines, fmt.Sprintf("• %s: <b>%d</b>", category, summary.ByCategory[category]))
			}

			avg := summary.TotalAmount / float64(summary.Count)
			msg := fmt.Sprintf(
				"<b>📦 10 Dakikalık Teklif Özeti</b>\n\n"+
					"Verilen teklif: <b>%d</b>\n"+
					"Toplam tutar: <b>$%.2f</b>\n"+
					"Ortalama teklif: <b>$%.2f</b>\n\n"+
					"<b>Kategori dağılımı</b>\n%s",
				summary.Count, summary.TotalAmount, avg, strings.Join(lines, "\n"))

			if err := r.tg.SendMessage(ctx, msg); err != nil {
				r.log.Errorf("telegram summary send failed: %v", err)
			}
		}
	}
}

func (r *Runner) runBuyerReplyNotifyLoop(ctx context.Context) {
	if r.cfg.TelegramBotToken == "" || r.cfg.TelegramChatID == 0 {
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	r.log.Infof("buyer reply notify loop started (interval=30s)")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.checkBuyerRepliesOnce(ctx); err != nil {
				r.log.Errorf("buyer reply notify loop error: %v", err)
			}
		}
	}
}

func (r *Runner) checkBuyerRepliesOnce(ctx context.Context) error {
	page, err := r.eld.ListReceivedBoostingRequests(ctx, eldorado.FilterOfferSubmitted, r.cfg.ValorantGameID)
	if err != nil {
		return err
	}

	for _, item := range page.Results {
		tr, ok := r.storage.GetTrackedOrder(item.ID)
		if !ok {
			continue
		}
		if r.storage.IsBuyerReplyNotified(item.ID) {
			continue
		}
		if !looksLikeBuyerReplied(item) {
			continue
		}

		rankInfo := ""
		if tr.CurrentRank != "" && tr.DesiredRank != "" {
			rankInfo = fmt.Sprintf("\nRank: <b>%s</b> ➜ <b>%s</b>", tr.CurrentRank, tr.DesiredRank)
		}
		text := fmt.Sprintf(
			"<b>💬 Müşteri Mesaj Attı (İlk Bildirim)</b>\n\n"+
				"Alici: %s\n"+
				"Kategori: %s\n"+
				"Fiyat: <b>$%.2f</b>%s\n"+
				"Request ID: <code>%s</code>\n"+
				"Hızlı komut: <code>/open_request %s</code>",
			item.BuyerUsername, tr.CategoryTitle, tr.OfferPrice, rankInfo, item.ID, item.ID,
		)
		requestURL := strings.TrimRight(r.cfg.EldoradoBaseURL, "/") + "/boosting-request/" + item.ID
		if err := r.tg.SendMessageWithURLButton(ctx, r.cfg.TelegramChatID, text, "🔗 Request'i Aç", requestURL); err != nil {
			r.log.Errorf("telegram buyer-reply send failed: %v", err)
			continue
		}
		if err := r.storage.MarkBuyerReplyNotified(item.ID); err != nil {
			r.log.Errorf("mark buyer reply notified failed (id=%s): %v", item.ID, err)
		}
	}

	return nil
}

func looksLikeBuyerReplied(item eldorado.BoostingRequestListItem) bool {
	if item.UnreadMessagesCount > 0 || item.HasUnreadMessages {
		return true
	}
	if item.LastMessageSenderID != "" && item.BuyerID != "" && item.LastMessageSenderID == item.BuyerID {
		return true
	}
	role := strings.ToLower(strings.TrimSpace(item.LastMessageSenderRole))
	return strings.Contains(role, "buyer")
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

		// Do not send per-offer Telegram message; queue it for 10-minute summary.
		r.recordOfferSummary(item.BoostingCategoryTitle, result.Price)

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

func (r *Runner) buildBuyerAutoMessage(requestID string) string {
	_ = requestID
	// TalkJS list formatu: satır başında - * veya + ile madde işareti oluşur, satır sonları korunur
	return `Hi! 👋 I provide professional Valorant boosting services for all ranks.

- ✔ Free Stream and Agent selection
- ✔ 4+ Years of Boosting Experience
- ✔ Radiant Since Beta
- ✔ 100% Feedbacks
- ✔ High K/D & Consistent Performance
- ✔ 100% Legit – No Cheats or Third-Party Programs
- ✔ Offline Mode Available (Discreet Service)
- ✔ No Voice or Text Communication

- ⭐ Free Stream
- ⭐ Free Agent Selection
- ⭐ Free Priority`
}

func (r *Runner) sendBuyerMessage(ctx context.Context, requestID string) {
	defer r.msgWg.Done()
	message := r.buildBuyerAutoMessage(requestID)
	if strings.TrimSpace(message) == "" {
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

	// Prefer TalkJS API (curl) when no image and token/nymId available
	tryAPI := strings.TrimSpace(r.cfg.BuyerAutoImage) == "" && strings.TrimSpace(r.cfg.TalkJsNymId) != ""
	if tryAPI {
		token := eldorado.LoadTalkJsTokenFromStorage(r.log)
		if token == "" {
			token = strings.TrimSpace(r.cfg.TalkJsToken)
		}
		if token == "" {
			if t, err := r.eld.TryGetTalkJsTokenForRequest(ctx, requestID); err == nil && t != "" {
				token = t
				eldorado.SaveTalkJsTokenToStorage(token, eldorado.JwtExp(token), r.log)
			} else if t, err := r.eld.TryGetTalkJsToken(ctx); err == nil && t != "" {
				token = t
				eldorado.SaveTalkJsTokenToStorage(token, eldorado.JwtExp(token), r.log)
			}
		}
		if token != "" {
			// Kısa bekleme: yeni konuşma TalkJS'te senkronize olsun (500 hatası önlemi)
			time.Sleep(3 * time.Second)

			// oneOnOneId(buyer,seller) = TalkJS conv ID (20 hex). Try API ids first, then config nym.
			buyerID := strings.TrimSpace(conv.BuyerUserID)
			sellerID := strings.TrimSpace(conv.SellerUserID)
			altID := eldorado.OneOnOneID(buyerID, r.cfg.TalkJsNymId)
			if altID == "" && buyerID != "" && sellerID != "" {
				altID = eldorado.OneOnOneID(buyerID, sellerID)
			}
			if altID == "" && buyerID != "" && !strings.HasSuffix(buyerID, "_n") {
				altID = eldorado.OneOnOneID(buyerID+"_n", r.cfg.TalkJsNymId)
			}
			if altID != "" {
				r.log.Infof("TalkJS oneOnOneId(buyer=%s,seller=%s)=%s", buyerID, sellerID, altID)
			}
			err := r.eld.TalkJsSayWithAlt(ctx, conv.TalkJsConversationID, altID, message, token, r.cfg.TalkJsNymId, r.log)
			if err == nil {
				r.storage.IncrementMessagesSent()
				r.resetMsgFailCount()
				r.log.Infof("buyer auto-message sent via API (requestId=%s)", requestID[:min(len(requestID), 12)])
				return
			}
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				eldorado.InvalidateTalkJsTokenStorage(r.log)
				// retry once with fresh token from API
				if t, e := r.eld.TryGetTalkJsToken(ctx); e == nil && t != "" {
					eldorado.SaveTalkJsTokenToStorage(t, eldorado.JwtExp(t), r.log)
					if r.eld.TalkJsSayWithAlt(ctx, conv.TalkJsConversationID, altID, message, t, r.cfg.TalkJsNymId, r.log) == nil {
						r.storage.IncrementMessagesSent()
						r.resetMsgFailCount()
						r.log.Infof("buyer auto-message sent via API after token refresh (requestId=%s)", requestID[:min(len(requestID), 12)])
						return
					}
				}
			}
			r.log.Infof("TalkJS API send failed, falling back to browser: %s", err.Error())
		}
	}

	if err := eldorado.SendChatMessage(ctx, requestID, message, r.cfg.BuyerAutoImage, conv.TalkJsConversationID, r.log); err != nil {
		r.log.Errorf("send buyer message failed (requestId=%s): %v", requestID, err)
		r.trackMsgError(ctx, requestID, err)
		return
	}
	r.storage.IncrementMessagesSent()
	r.resetMsgFailCount()
	r.log.Infof("buyer auto-message sent successfully (requestId=%s)", requestID[:min(len(requestID), 12)])
}
