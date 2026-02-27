package eldorado

// --- Boosting Request Listing ---

type BoostingRequestListItem struct {
	ID                    string `json:"id"`
	GameID                string `json:"gameId"`
	BoostingCategoryID    string `json:"boostingCategoryId"`
	BoostingCategoryTitle string `json:"boostingCategoryTitle"`
	CreatedDate           string `json:"createdDate"`
	BuyerID               string `json:"buyerId"`
	BuyerUsername         string `json:"buyerUsername"`
	IsBuyerMuted          bool   `json:"isBuyerMuted"`
	HasUnreadMessages     bool   `json:"hasUnreadMessages"`
	UnreadMessagesCount   int    `json:"unreadMessagesCount"`
	LastMessageSenderID   string `json:"lastMessageSenderId"`
	LastMessageSenderRole string `json:"lastMessageSenderRole"`
}

type BoostingRequestPage struct {
	Results    []BoostingRequestListItem `json:"results"`
	PageSize   int                       `json:"pageSize"`
	NextCursor string                    `json:"nextPageCursor"`
}

// --- Boosting Request Detail (GET /api/boostingOffers/boostingRequests/{id}) ---

type DescriptionValue struct {
	ID                    int    `json:"id"`
	Value                 string `json:"value"`
	IncludeInNotification bool   `json:"includeInNotification"`
	Label                 string `json:"label"`
}

type BoostingRequestDetails struct {
	DescriptionValues []DescriptionValue `json:"descriptionValues"`
	DesiredPrice      *MoneyBase         `json:"desiredPrice"`
}

type BoostingRequestFull struct {
	ID                     string                  `json:"id"`
	GameID                 string                  `json:"gameId"`
	BoostingCategoryID     string                  `json:"boostingCategoryId"`
	GameCategoryTitle      string                  `json:"gameCategoryTitle"`
	GameName               string                  `json:"gameName"`
	State                  string                  `json:"state"`
	UserID                 string                  `json:"userId"`
	CreatedDate            string                  `json:"createdDate"`
	IsBuyerMuted           bool                    `json:"isBuyerMuted"`
	BoostingRequestDetails *BoostingRequestDetails `json:"boostingRequestDetails"`
	OffersCount            int                     `json:"offersCount"`
	AvailableSellersCount  int                     `json:"availableSellersCount"`
}

// Helper to extract a description value by label
func (r *BoostingRequestFull) GetDescValue(label string) string {
	if r.BoostingRequestDetails == nil {
		return ""
	}
	for _, dv := range r.BoostingRequestDetails.DescriptionValues {
		if dv.Label == label {
			return dv.Value
		}
	}
	return ""
}

// --- Create Boosting Offer ---

type MoneyBase struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type OfferPricing struct {
	Quantity     int       `json:"quantity"`
	MinQuantity  int       `json:"minQuantity"`
	PricePerUnit MoneyBase `json:"pricePerUnit"`
}

type BoostingOfferDetails struct {
	BoostingRequestID      string       `json:"boostingRequestId"`
	GuaranteedDeliveryTime string       `json:"guaranteedDeliveryTime"`
	Pricing                OfferPricing `json:"pricing"`
}

type BoostingOfferPost struct {
	Details BoostingOfferDetails `json:"details"`
}

// --- Boosting Offer Response ---

type BoostingOfferPublic struct {
	ID                 string `json:"id"`
	UserID             string `json:"userId"`
	GameID             string `json:"gameId"`
	BoostingRequestID  string `json:"boostingRequestId"`
	BoostingCategoryID string `json:"boostingCategoryId"`
	OfferState         string `json:"offerState"`
}

// --- Boosting Subscriptions ---

type BoostingSubscription struct {
	GameID               string `json:"gameId"`
	GameName             string `json:"gameName"`
	BoostingCategoryID   string `json:"boostingCategoryId"`
	BoostingCategoryName string `json:"boostingCategoryName"`
	IsActive             bool   `json:"isActive"`
	IsSubscribed         bool   `json:"isSubscribed"`
}

// --- Conversation ---

type BoostingConversation struct {
	BoostingRequestID    string `json:"boostingRequestId"`
	TalkJsConversationID string `json:"talkJsConversationId"`
	BuyerUserID          string `json:"buyerUserId"`
	SellerUserID         string `json:"sellerUserId"`
}

// --- Orders ---

type StateDTO struct {
	State       string `json:"state"`
	CreatedDate string `json:"createdDate"`
}

type OrderOfferDetails struct {
	GameID            string `json:"gameId"`
	GameName          string `json:"gameName"`
	GameCategoryTitle string `json:"gameCategoryTitle"`
}

type OrderPrivate struct {
	ID                string             `json:"id"`
	SellerID          string             `json:"sellerId"`
	BuyerID           string             `json:"buyerId"`
	OfferID           string             `json:"offerId"`
	State             StateDTO           `json:"state"`
	OrderOfferDetails *OrderOfferDetails `json:"orderOfferDetails"`
	BuyerUsername     string             `json:"buyerUsername"`
	SellerUsername    string             `json:"sellerUsername"`
	CreatedDate       string             `json:"createdDate"`
	TotalPrice        *MoneyBase         `json:"totalPrice"`
}

type OrderPage struct {
	Results    []OrderPrivate `json:"results"`
	TotalCount int            `json:"totalCount"`
}

// Boosting request seller filter enum values
const (
	FilterActiveRequests = "ActiveRequests"
	FilterOfferSubmitted = "OfferSubmitted"
	FilterOfferWon       = "OfferWon"
	FilterOfferLost      = "OfferLost"
)

// Guaranteed delivery time enum values (Eldorado fixed slots)
const (
	DeliveryHour1  = "Hour1"  // 1h
	DeliveryHour2  = "Hour2"  // 2h
	DeliveryHour3  = "Hour3"  // 3h
	DeliveryHour5  = "Hour5"  // 5h
	DeliveryHour8  = "Hour8"  // 8h
	DeliveryHour12 = "Hour12" // 12h
	DeliveryDay1   = "Day1"   // 24h
	DeliveryDay2   = "Day2"   // 48h
	DeliveryDay3   = "Day3"   // 72h
	DeliveryDay7   = "Day7"   // 168h
	DeliveryDay14  = "Day14"  // 336h
	DeliveryDay28  = "Day28"  // 672h
)

// Boosting category IDs for Valorant
const (
	CategoryRankBoost     = "0"
	CategoryPlacements    = "1"
	CategoryNetWins       = "2"
	CategoryCustomRequest = "3"
	CategoryCoaching      = "8"
	CategoryRadiantBoost  = "9"
)
