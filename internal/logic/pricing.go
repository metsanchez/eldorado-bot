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

// Hours per division by tier: Iron–Plat 4h, Ascendant 7h, Immortal 24h
var hoursPerDivisionByTier = map[string]float64{
	"iron": 4, "bronze": 4, "silver": 4, "gold": 4, "platinum": 4,
	"diamond": 4, "ascendant": 7, "immortal": 24,
}

func hoursForDivision(rank string) float64 {
	if h, ok := hoursPerDivisionByTier[rankTier(rank)]; ok {
		return h
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

// Point order price per tier (RR-based requests)
var pointPrice = map[string]float64{
	"iron": 1, "bronze": 1, "silver": 1, "gold": 2,
	"platinum": 3, "diamond": 4, "ascendant": 5, "immortal": 10,
}

// Flat price per division (from rank X to next division)
var divisionPrice = map[string]float64{
	"Iron I": 3, "Iron II": 3, "Iron III": 3,
	"Bronze I": 3, "Bronze II": 3, "Bronze III": 3,
	"Silver I": 3, "Silver II": 4, "Silver III": 4,
	"Gold I": 4, "Gold II": 4, "Gold III": 4,
	"Platinum I": 5, "Platinum II": 5, "Platinum III": 8,
	"Diamond I": 10, "Diamond II": 10, "Diamond III": 11,
	"Ascendant I": 15, "Ascendant II": 17, "Ascendant III": 19,
	"Immortal I": 33, "Immortal II": 65, "Immortal III": 50,
}

// rrDiscountForFirstDivision returns discount $ for Diamond+ based on current RR (0-99).
// Diamond: $1.5 per 25 RR, Ascendant: $2 per 25 RR, Immortal: $5 per 25 RR.
func rrDiscountForFirstDivision(tier string, currentRR float64) float64 {
	if currentRR <= 0 {
		return 0
	}
	var per25 float64
	switch tier {
	case "diamond":
		per25 = 1.5
	case "ascendant":
		per25 = 2
	case "immortal":
		per25 = 5
	default:
		return 0
	}
	// Every full 25 RR block gives discount (25-49=1, 50-74=2, 75-99=3)
	blocks := math.Floor(currentRR / 25)
	return blocks * per25
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
	method = strings.ToLower(strings.TrimSpace(method))

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
// Delivery time = sum of hours per division (Iron–Plat 4h, Ascendant 7h, Immortal 24h), mapped to Eldorado slots.
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

	var totalPrice float64

	for i := fromIdx; i < toIdx; i++ {
		cost, ok := divisionPrice[rankOrder[i]]
		if !ok {
			return PriceResult{Skip: true, SkipReason: "no price for division: " + rankOrder[i]}
		}
		// Diamond+: first division only — discount per 25 RR (Diamond $1.5, Ascendant $2, Immortal $5)
		if i == fromIdx && currentRR > 0 {
			tier := rankTier(rankOrder[i])
			if disc := rrDiscountForFirstDivision(tier, currentRR); disc > 0 {
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
		totalHours += hoursForDivision(rankOrder[i])
	}
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

	price, ok2 := pointPrice[tier]
	if !ok2 {
		return PriceResult{Skip: true, SkipReason: "no point pricing for tier: " + tier}
	}
	delivery := hoursToDeliveryTime(hoursForDivision(currentRank))

	return PriceResult{Price: price * multiplier, DeliveryTime: delivery, Method: methodName}
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
