package subscription

const (
	SubscriptionStatusPending      SubscriptionStatus = "pending"
	SubscriptionStatusActive       SubscriptionStatus = "active"
	SubscriptionStatusUnsubscribed SubscriptionStatus = "unsubscribed"
)

type SubscriptionStatus string

type Subscription struct {
	Email       string
	Repository  string
	Confirmed   bool
	LastSeenTag string
}
