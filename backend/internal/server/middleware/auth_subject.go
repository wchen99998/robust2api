package middleware

import "github.com/gin-gonic/gin"

// AuthSubject is the minimal authenticated identity stored in gin context.
// UserID remains the linked app-domain user id for compatibility with existing handlers.
type AuthSubject struct {
	SubjectID    string
	SessionID    string
	UserID      int64
	Concurrency int
}

func GetAuthSubjectFromContext(c *gin.Context) (AuthSubject, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return AuthSubject{}, false
	}
	subject, ok := value.(AuthSubject)
	return subject, ok
}

func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyUserRole))
	if !exists {
		return "", false
	}
	role, ok := value.(string)
	return role, ok
}
