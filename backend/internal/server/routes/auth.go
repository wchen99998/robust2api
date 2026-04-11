package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterAuthRoutes registers the control-plane browser auth/profile API.
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.ControlHandlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
) {
	rateLimiter := middleware.NewRateLimiter(redisClient)

	v1.GET("/bootstrap", h.Auth.Bootstrap)

	public := v1.Group("")
	public.Use(servermiddleware.BackendModeAuthGuard(settingService))
	{
		session := public.Group("/session")
		{
			session.POST("/login", rateLimiter.LimitWithOptions("control-session-login", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionLogin)
			session.POST("/login/totp", rateLimiter.LimitWithOptions("control-session-login-totp", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionLoginTOTP)
			session.POST("/refresh", rateLimiter.LimitWithOptions("control-session-refresh", 30, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionRefresh)
		}

		registration := public.Group("/registration")
		{
			registration.POST("/preflight", rateLimiter.LimitWithOptions("control-registration-preflight", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationPreflight)
			registration.POST("/email-code", rateLimiter.LimitWithOptions("control-registration-email-code", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationEmailCode)
			registration.POST("", rateLimiter.LimitWithOptions("control-registration", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.Registration)
			registration.POST("/complete", rateLimiter.LimitWithOptions("control-registration-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationComplete)
		}

		password := public.Group("/password")
		{
			password.POST("/forgot", rateLimiter.LimitWithOptions("control-password-forgot", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.PasswordForgot)
			password.POST("/reset", rateLimiter.LimitWithOptions("control-password-reset", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.PasswordReset)
		}

		oauth := public.Group("/oauth")
		{
			oauth.GET("/:provider/start", h.Auth.OAuthStart)
			oauth.GET("/:provider/callback", h.Auth.OAuthCallback)
		}
	}

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(servermiddleware.BackendModeUserGuard(settingService))
	{
		authenticated.DELETE("/session", h.Auth.SessionLogout)
		authenticated.DELETE("/sessions", h.Auth.SessionsLogoutAll)
		authenticated.POST("/embed-token", h.Auth.EmbedToken)
		authenticated.PATCH("/me", h.Auth.PatchMe)
		authenticated.POST("/me/password/change", h.Auth.ChangeMyPassword)

		totp := authenticated.Group("/me/mfa/totp")
		{
			totp.GET("", h.Auth.GetMyTOTP)
			totp.POST("/send-code", h.Auth.SendMyTOTPCode)
			totp.POST("/setup", h.Auth.SetupMyTOTP)
			totp.POST("/enable", h.Auth.EnableMyTOTP)
			totp.DELETE("", h.Auth.DisableMyTOTP)
		}
	}
}
