package logic

import (
	"eldorado-bot/internal/config"
	"eldorado-bot/internal/eldorado"
)

// ShouldBidOn decides whether to bid on a received boosting request.
// Returns true + offer price if we should bid.
func ShouldBidOn(req *eldorado.BoostingRequestListItem, cfg *config.Config) (bool, float64) {
	// Filter by game ID if configured
	if cfg.ValorantGameID != "" && req.GameID != cfg.ValorantGameID {
		return false, 0
	}

	// Skip muted buyers
	if req.IsBuyerMuted {
		return false, 0
	}

	price := cfg.MinOfferPrice
	if price <= 0 {
		return false, 0
	}

	if cfg.MaxOfferPrice > 0 && price > cfg.MaxOfferPrice {
		return false, 0
	}

	return true, price
}
