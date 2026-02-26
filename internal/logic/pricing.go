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

const hoursPerDivision = 4

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

// Average RR gained per game by tier
var rrPerGame = map[string]float64{
	"iron": 22, "bronze": 22, "silver": 22, "gold": 22,
	"platinum": 22, "diamond": 20, "ascendant": 18, "immortal": 17,
}

// Price charged per game by tier
var pricePerGame = map[string]float64{
	"iron": 1, "bronze": 1, "silver": 1, "gold": 2,
	"platinum": 3, "diamond": 4, "ascendant": 5, "immortal": 10,
}

// divisionCostByRR calculates cost to gain rrNeeded RR at a given rank.
// games = ceil(rrNeeded / rrPerGame), cost = games * pricePerGame
func divisionCostByRR(rank string, rrNeeded float64) (float64, bool) {
	tier := rankTier(rank)
	rr, ok1 := rrPerGame[tier]
	price, ok2 := pricePerGame[tier]
	if !ok1 || !ok2 || rr <= 0 {
		return 0, false
	}
	games := math.Ceil(rrNeeded / rr)
	return games * price, true
}

// Net win prices per rank tier
var netWinPrice = map[string]float64{
	"iron": 5, "bronze": 5, "silver": 5, "gold": 5,
	"platinum": 5, "diamond": 5, "ascendant": 8, "immortal": 11,
}

func netWinPriceForRank(rank string) float64 {
	r := normalizeRank(rank)
	tier := rankTier(r)

	switch r {
	case "Immortal I":
		return 11
	case "Ascendant I", "Ascendant II", "Ascendant III":
		return 8
	}

	if p, ok := netWinPrice[tier]; ok {
		return p
	}
	return 5
}

// checkServerAndMethod validates server=EU and returns the duo multiplier (1 for Solo, 2 for Duo).
func checkServerAndMethod(req *eldorado.BoostingRequestFull) (multiplier float64, methodName string, skip PriceResult, ok bool) {
	server := strings.TrimSpace(req.GetDescValue("Server"))
	if server == "" {
		server = strings.TrimSpace(req.GetDescValue("server"))
	}
	if !strings.EqualFold(server, "EU") {
		return 0, "", PriceResult{Skip: true, SkipReason: "non-EU server: " + server}, false
	}

	method := strings.TrimSpace(req.GetDescValue("Completion Method"))
	if method == "" {
		method = strings.TrimSpace(req.GetDescValue("completion method"))
	}
	method = strings.ToLower(method)

	switch method {
	case "duo":
		return 2, "Duo", PriceResult{}, true
	default:
		return 1, "Solo", PriceResult{}, true
	}
}

// CalculateRankBoostPrice calculates total price for a rank boost from currentRank to desiredRank.
// Delivery time = number of divisions * 4 hours.
func CalculateRankBoostPrice(req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValue("Current Rank"))
	desiredRank := normalizeRank(req.GetDescValue("Desired rank"))
	currentRRStr := req.GetDescValue("Current RR")

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

	divisions := toIdx - fromIdx
	var totalPrice float64

	// First division: only need (100 - currentRR) to rank up
	firstDivRR := 100.0 - currentRR
	cost, ok := divisionCostByRR(rankOrder[fromIdx], firstDivRR)
	if !ok {
		return PriceResult{Skip: true, SkipReason: "no RR pricing for: " + rankOrder[fromIdx]}
	}
	totalPrice += cost

	// Subsequent divisions: full 100 RR each
	for i := fromIdx + 1; i < toIdx; i++ {
		cost, ok := divisionCostByRR(rankOrder[i], 100.0)
		if !ok {
			return PriceResult{Skip: true, SkipReason: "no RR pricing for: " + rankOrder[i]}
		}
		totalPrice += cost
	}

	totalPrice *= multiplier
	totalHours := float64(divisions) * hoursPerDivision
	delivery := hoursToDeliveryTime(totalHours)

	return PriceResult{Price: totalPrice, DeliveryTime: delivery, Method: methodName}
}

// CalculateNetWinPrice calculates price for net win orders.
// Delivery time = number of games * 4 hours.
func CalculateNetWinPrice(req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValue("Current season rank"))
	if currentRank == "" {
		currentRank = normalizeRank(req.GetDescValue("Current Rank"))
	}
	numGamesStr := req.GetDescValue("Number of games")

	if currentRank == "" {
		return PriceResult{Skip: true, SkipReason: "missing rank for net win"}
	}

	if rankTier(currentRank) == "immortal" {
		rrStr := req.GetDescValue("Current RR")
		rr, _ := strconv.Atoi(rrStr)
		if rr >= 300 {
			return PriceResult{Skip: true, SkipReason: "Immortal 300+ skip"}
		}
	}

	pricePerWin := netWinPriceForRank(currentRank)
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
// Uses the same per-game pricing: Iron/Bronze/Silver $1, Gold $2, Plat $3,
// Diamond $4, Ascendant $5, Immortal $10.
func CalculatePointPrice(req *eldorado.BoostingRequestFull) PriceResult {
	multiplier, methodName, skipResult, ok := checkServerAndMethod(req)
	if !ok {
		return skipResult
	}

	currentRank := normalizeRank(req.GetDescValue("Current season rank"))
	if currentRank == "" {
		currentRank = normalizeRank(req.GetDescValue("Current Rank"))
	}

	if currentRank == "" {
		return PriceResult{Skip: true, SkipReason: "missing rank for points"}
	}

	tier := rankTier(currentRank)

	if tier == "immortal" {
		rrStr := req.GetDescValue("Current RR")
		rr, _ := strconv.Atoi(rrStr)
		if rr >= 300 {
			return PriceResult{Skip: true, SkipReason: "Immortal 300+ skip"}
		}
	}

	price, ok2 := pricePerGame[tier]
	if !ok2 {
		return PriceResult{Skip: true, SkipReason: "no point pricing for tier: " + tier}
	}
	delivery := hoursToDeliveryTime(hoursPerDivision)

	return PriceResult{Price: price * multiplier, DeliveryTime: delivery, Method: methodName}
}

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
	default:
		return eldorado.DeliveryDay3
	}
}
