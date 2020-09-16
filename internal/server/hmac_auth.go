package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"fmt"
	"io/ioutil"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type (
	// HmacAuthConfig defines the config for HmacAuth middleware.
	HmacAuthConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper middleware.Skipper

		// Validator is a function to validate HmacAuth hash.
		// Required.
		Validator HmacAuthValidator

		// Secret is a string that contains the secret to build the hash.
		Secret string
	}

	// HmacAuthValidator defines a function to validate HmacAuth credentials.
	HmacAuthValidator func([]byte, string, string, echo.Context) (bool, error)
)

var (
	// DefaultHmacAuthConfig is the default HmacAuth middleware config.
	DefaultHmacAuthConfig = HmacAuthConfig{
		Skipper:   middleware.DefaultSkipper,
		Validator: DefaultHmacAuthValidator,
	}
)

// HmacAuth returns an HmacAuth middleware.
//
// For valid hash it calls the next handler.
// For missing or invalid hash, it sends "403 - Forbidden" response.
func HmacAuth(secret string) echo.MiddlewareFunc {
	c := DefaultHmacAuthConfig
	c.Validator = DefaultHmacAuthValidator
	c.Secret = secret
	return HmacAuthWithConfig(c)
}

// HmacAuthWithConfig returns an HmacAuth middleware with config.
// See `HmacAuth()`.
func HmacAuthWithConfig(config HmacAuthConfig) echo.MiddlewareFunc {
	// Defaults
	if config.Validator == nil {
		config.Validator = DefaultHmacAuthConfig.Validator
	}
	if config.Skipper == nil {
		config.Skipper = DefaultHmacAuthConfig.Skipper
	}
	if len(config.Secret) < 1 {
		panic("echo: hmac-auth middleware requires a secret")
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			signature := c.Request().Header.Get("X-Hub-Signature")

			// Request
			reqBody := []byte{}
			if c.Request().Body != nil { // Read
				reqBody, _ = ioutil.ReadAll(c.Request().Body)
			}
			c.Request().Body = ioutil.NopCloser(bytes.NewBuffer(reqBody)) // Reset

			valid, err := config.Validator(reqBody, config.Secret, signature, c)
			if err != nil {
				return err
			} else if valid {
				return next(c)
			}

			return echo.ErrForbidden
		}
	}
}

func DefaultHmacAuthValidator(payload []byte, signature string, secret string, c echo.Context) (bool, error) {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)

	return fmt.Sprintf("sha1=%x", mac.Sum(nil)) == signature, nil
}
