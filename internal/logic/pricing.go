package logic

import (
	"math"
	"strconv"
	"strings"

	"eldorado-bot/internal/eldorado"
)

type PriceResult struct {
	Price        float64
	DeliveryTime string
	Method       string // "Solo" or "Duo"
	Skip         bool
	SkipReason   string
}

// Valorant rank order: 8 ranks x 3 divisions + Radiant = 25 tiers
var rankOrder = []string{
	"Iron I", "Iron II", "Iron III",
	"Bronze I", "Bronze II", "Bronze III",
	"Silver I", "Silver II", "Silver III",
	"Gold I", "Gold II", "Gold III",
	"Platinum I", "Platinum II", "Platinum III",
	"Diamond I", "Diamond II", "Diamond III",
	"Ascendant I", "Ascendant II", "Ascendant III",
	"Immortal I", "Immortal II", "Immortal III",
	"Radiant",
}

func hoursForDivision(cfg *PriceConfig, rank string) float64 {
	if cfg != nil && cfg.HoursPerDivision != nil {
		if h, ok := cfg.HoursPerDivision[rankTier(rank)]; ok {
			return h
		}
	}
	return 4
}

func rankIndex(rank string) int {
	r := normalizeRank(rank)
	for i, v := range rankOrder {
		if strings.EqualFold(v, r) {
			return i
		}
	}
	return -1
}

func normalizeRank(rank string) string {
	r := strings.TrimSpace(rank)
	r = strings.ReplaceAll(r, "Plat ", "Platinum ")
	r = strings.ReplaceAll(r, "PLAT ", "Platinum ")
	r = strings.ReplaceAll(r, "IRON ", "Iron ")
	r = strings.ReplaceAll(r, "BRONZE ", "Bronze ")
	r = strings.ReplaceAll(r, "SILVER ", "Silver ")
	r = strings.ReplaceAll(r, "GOLD ", "Gold ")
	r = strings.ReplaceAll(r, "DIAMOND ", "Diamond ")
	r = strings.ReplaceAll(r, "ASCENDANT ", "Ascendant ")
	r = strings.ReplaceAll(r, "IMMORTAL ", "Immortal ")
	r = strings.ReplaceAll(r, "RADIANT", "Radiant")
	return r
}

func rankTier(rank string) string {
	r := strings.ToLower(normalizeRank(rank))
	switch {
	case strings.HasPrefix(r, "iron"):
		return "iron"
	case strings.HasPrefix(r, "bronze"):
		return "bronze"
	case strings.HasPrefix(r, "silver"):
		return "silver"
	case strings.HasPrefix(r, "gold"):
		return "gold"
	case strings.HasPrefix(r, "platinum"):
		return "platinum"
	case strings.HasPrefix(r, "diamond"):
		return "diamond"
	case strings.HasPrefix(r, "ascendant"):
		return "ascendant"
	case strings.HasPrefix(r, "immortal"):
		return "immortal"
	case strings.HasPrefix(r, "radiant"):
		return "radiant"
	}
	return ""
}

func rrDiscountForFirstDivision(cfg *PriceConfig, tier string, currentRR float64) float64 {
	if currentRR <= 0 || cfg == nil || cfg.RRDiscountPer25 == nil {
		return 0
	}
	per25, ok := cfg.RRDiscountPer25[tier]
	if !ok || per25 <= 0 {
		return 0
	}
	blocks := math.Floor(currentRR / 25)
	return blocks * per25
}

func netWinPriceForRank(cfg *PriceConfig, rank string) float64 {
	r := normalizeRank(rank)
	if cfg != nil && cfg.NetWinOverride != nil {
		if p, ok := cfg.NetWinOverride[r]; ok {
			return p
		}
	}
	tier := rankTier(r)
	if cfg != nil && cfg.NetWinPrice != nil {
		if p, ok := cfg.NetWinPrice[tier]; ok {
			return p
		}
	}
	return 5
}

// checkServerAndMethod validates server=EU and returns the duo multiplier (1 for Solo, 2 for Duo).
func checkServerAndMethod(req *eldorado.BoostingRequestFull) (multiplier float64, methodName string, skip PriceResult, ok bool) {
	server := strings.TrimSpace(req.GetDescValueMulti("Server", "server", "Region"))
	if server == "" {
		return 0, "", PriceResult{Skip: true, SkipReason: "missing server/region info"}, false
	}
	if !strings.EqualFold(server, "EU") {
		return 0, "", PriceResult{Skip: true, SkipReason: "non-EU server: " + server}, false
	}

	method := strings.ToLower(strings.TrimSpace(req.GetDescValueMulti("Completion Method", "completion method", "Method")))

	switch method {
	case "duo":
		return 2, "Duo", PriceResult{}, true
	}
	// Be tolerant to API text variants like "Duo Boost", "Duo Queue", etc.
	if strings.Contains(method, "duo") {
		return 2, "Duo", PriceResult{}, true
	}
	return 1, "Solo", PriceResult{}, true
}

// CalculateRankBoostPrice calculates total price for a rank boost from currentRank to desiredRank.
// cfg can be nil to use built-in defaults.
func CalculateRankBoostPrice(cfg *PriceConfig, req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValueMulti("Current Rank", "From Rank", "Current rank"))
	desiredRank := normalizeRank(req.GetDescValueMulti("Desired rank", "Desired Rank", "To Rank", "Target rank"))
	currentRRStr := req.GetDescValueMulti("Current RR", "Current rr", "RR")

	if currentRank == "" || desiredRank == "" {
		return PriceResult{Skip: true, SkipReason: "missing rank info"}
	}

	fromIdx := rankIndex(currentRank)
	toIdx := rankIndex(desiredRank)
	if fromIdx < 0 || toIdx < 0 {
		return PriceResult{Skip: true, SkipReason: "unknown rank: " + currentRank + " or " + desiredRank}
	}
	if fromIdx >= toIdx {
		return PriceResult{Skip: true, SkipReason: "current rank >= desired rank"}
	}

	if rankTier(currentRank) == "immortal" {
		rr, _ := strconv.Atoi(currentRRStr)
		if rr >= 300 {
			return PriceResult{Skip: true, SkipReason: "Immortal 300+ skip"}
		}
	}

	if rankTier(desiredRank) == "radiant" {
		return PriceResult{Skip: true, SkipReason: "Radiant boost skip"}
	}

	// Parse current RR — higher RR means closer to next division, so first division is cheaper
	currentRR := 0.0
	if currentRRStr != "" {
		if rr, err := strconv.ParseFloat(currentRRStr, 64); err == nil && rr > 0 && rr < 100 {
			currentRR = rr
		}
	}

	divPrice := getDivisionPrice(cfg)
	var totalPrice float64

	for i := fromIdx; i < toIdx; i++ {
		cost, ok := divPrice[rankOrder[i]]
		if !ok {
			return PriceResult{Skip: true, SkipReason: "no price for division: " + rankOrder[i]}
		}
		if i == fromIdx && currentRR > 0 {
			tier := rankTier(rankOrder[i])
			if disc := rrDiscountForFirstDivision(cfg, tier, currentRR); disc > 0 {
				cost -= disc
				if cost < 0 {
					cost = 0
				}
			}
		}
		totalPrice += cost
	}

	totalPrice *= multiplier
	var totalHours float64
	for i := fromIdx; i < toIdx; i++ {
		totalHours += hoursForDivision(cfg, rankOrder[i])
	}
	delivery := hoursToDeliveryTime(totalHours)

	return PriceResult{Price: totalPrice, DeliveryTime: delivery, Method: methodName}
}

func getDivisionPrice(cfg *PriceConfig) map[string]float64 {
	if cfg != nil && cfg.DivisionPrice != nil {
		return cfg.DivisionPrice
	}
	return defaultPriceConfig().DivisionPrice
}

// CalculateNetWinPrice calculates price for net win orders.
func CalculateNetWinPrice(cfg *PriceConfig, req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValueMulti("Current season rank", "Current Rank", "From Rank"))
	numGamesStr := req.GetDescValueMulti("Number of games", "Number Of Games", "Games")

	if currentRank == "" {
		return PriceResult{Skip: true, SkipReason: "missing rank for net win"}
	}

	if rankTier(currentRank) == "immortal" {
		rrStr := req.GetDescValueMulti("Current RR", "Current rr", "RR")
		rr, _ := strconv.Atoi(rrStr)
		if rr >= 300 {
			return PriceResult{Skip: true, SkipReason: "Immortal 300+ skip"}
		}
	}

	pricePerWin := netWinPriceForRank(cfg, currentRank)
	numGames := 1
	if n, err := strconv.Atoi(numGamesStr); err == nil && n > 0 {
		numGames = n
	}

	totalPrice := pricePerWin * float64(numGames) * multiplier
	totalHours := float64(numGames) * 2
	delivery := hoursToDeliveryTime(totalHours)

	return PriceResult{Price: totalPrice, DeliveryTime: delivery, Method: methodName}
}

// CalculatePointPrice calculates price for RR point-based orders.
func CalculatePointPrice(cfg *PriceConfig, req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValueMulti("Current season rank", "Current Rank", "From Rank"))

	if currentRank == "" {
		return PriceResult{Skip: true, SkipReason: "missing rank for points"}
	}

	tier := rankTier(currentRank)

	if tier == "immortal" {
		rrStr := req.GetDescValueMulti("Current RR", "Current rr", "RR")
		rr, _ := strconv.Atoi(rrStr)
		if rr >= 300 {
			return PriceResult{Skip: true, SkipReason: "Immortal 300+ skip"}
		}
	}

	pointPrice := getPointPrice(cfg)
	price, ok2 := pointPrice[tier]
	if !ok2 {
		return PriceResult{Skip: true, SkipReason: "no point pricing for tier: " + tier}
	}
	delivery := hoursToDeliveryTime(hoursForDivision(cfg, currentRank))

	return PriceResult{Price: price * multiplier, DeliveryTime: delivery, Method: methodName}
}

func getPointPrice(cfg *PriceConfig) map[string]float64 {
	if cfg != nil && cfg.PointPrice != nil {
		return cfg.PointPrice
	}
	return defaultPriceConfig().PointPrice
}

// hoursToDeliveryTime maps calculated hours to the nearest Eldorado slot (rounds up for safety)
func hoursToDeliveryTime(hours float64) string {
	switch {
	case hours <= 1:
		return eldorado.DeliveryHour1
	case hours <= 2:
		return eldorado.DeliveryHour2
	case hours <= 3:
		return eldorado.DeliveryHour3
	case hours <= 5:
		return eldorado.DeliveryHour5
	case hours <= 8:
		return eldorado.DeliveryHour8
	case hours <= 12:
		return eldorado.DeliveryHour12
	case hours <= 24:
		return eldorado.DeliveryDay1
	case hours <= 48:
		return eldorado.DeliveryDay2
	case hours <= 72:
		return eldorado.DeliveryDay3
	case hours <= 168:
		return eldorado.DeliveryDay7
	case hours <= 336:
		return eldorado.DeliveryDay14
	default:
		return eldorado.DeliveryDay28
	}
}
