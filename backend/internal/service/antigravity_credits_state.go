package service

const creditsExhaustedKey = "AICredits"

func (a *Account) isCreditsExhausted() bool {
	if a == nil {
		return false
	}
	return a.isRateLimitActiveForKey(creditsExhaustedKey)
}
