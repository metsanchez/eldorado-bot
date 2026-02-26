package logic

import (
	"testing"

	"eldorado-bot/internal/config"
	"eldorado-bot/internal/eldorado"
)

func TestShouldBidOn_MatchesGameID(t *testing.T) {
	cfg := &config.Config{
		ValorantGameID: "abc-123",
		MinOfferPrice:  10,
		MaxOfferPrice:  50,
	}

	req := &eldorado.BoostingRequestListItem{
		ID:     "req1",
		GameID: "abc-123",
	}

	ok, price := ShouldBidOn(req, cfg)
	if !ok {
		t.Fatalf("expected to bid, got false")
	}
	if price != 10 {
		t.Fatalf("expected price 10, got %.2f", price)
	}
}

func TestShouldBidOn_WrongGame(t *testing.T) {
	cfg := &config.Config{
		ValorantGameID: "abc-123",
		MinOfferPrice:  10,
	}

	req := &eldorado.BoostingRequestListItem{
		ID:     "req1",
		GameID: "different-game",
	}

	ok, _ := ShouldBidOn(req, cfg)
	if ok {
		t.Fatalf("expected not to bid for different game")
	}
}

func TestShouldBidOn_MutedBuyer(t *testing.T) {
	cfg := &config.Config{
		ValorantGameID: "abc-123",
		MinOfferPrice:  10,
	}

	req := &eldorado.BoostingRequestListItem{
		ID:           "req1",
		GameID:       "abc-123",
		IsBuyerMuted: true,
	}

	ok, _ := ShouldBidOn(req, cfg)
	if ok {
		t.Fatalf("expected not to bid on muted buyer")
	}
}

func TestShouldBidOn_PriceExceedsMax(t *testing.T) {
	cfg := &config.Config{
		ValorantGameID: "abc-123",
		MinOfferPrice:  100,
		MaxOfferPrice:  50,
	}

	req := &eldorado.BoostingRequestListItem{
		ID:     "req1",
		GameID: "abc-123",
	}

	ok, _ := ShouldBidOn(req, cfg)
	if ok {
		t.Fatalf("expected not to bid when price exceeds max")
	}
}
