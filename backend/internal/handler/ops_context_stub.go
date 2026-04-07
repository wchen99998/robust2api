package handler

import "github.com/gin-gonic/gin"

const (
	opsModelKey     = "ops_model"
	opsAccountIDKey = "ops_account_id"
)

// Legacy ops context helpers now only preserve the lightweight request metadata
// that other request-path logs and tests still read.
func setOpsRequestContext(c *gin.Context, model string, _ bool, _ []byte) {
	if c == nil || model == "" {
		return
	}
	c.Set(opsModelKey, model)
}

func setOpsEndpointContext(_ *gin.Context, _ string, _ int16) {}

func setOpsSelectedAccount(c *gin.Context, accountID int64, _ string) {
	if c == nil || accountID <= 0 {
		return
	}
	c.Set(opsAccountIDKey, accountID)
}
