// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package secure

import (
	"fmt"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/internal/shared"
)

// Config controls defensive browser response headers.
type Config struct {
	XSSProtection                   string
	ContentTypeNosniff              string
	XFrameOptions                   string
	HSTSMaxAge                      int
	HSTSExcludeSubdomains           bool
	ContentSecurityPolicy           string
	ContentSecurityPolicyReportOnly string
	ReferrerPolicy                  string
	CrossOriginResourcePolicy       string
	PermissionsPolicy               string
}

// defaultConfig returns conservative browser-security headers without HSTS.
func defaultConfig() Config {
	return Config{
		XSSProtection:             "0",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "SAMEORIGIN",
		ReferrerPolicy:            "no-referrer",
		CrossOriginResourcePolicy: "same-origin",
	}
}

// New applies security headers. HSTS is emitted only for requests Zinc
// considers secure, including trusted-proxy scheme handling.
func New(configs ...Config) zinc.Middleware {
	config := shared.Config("secure", configs)
	cfg := resolveSecureConfig(config)

	return func(c *zinc.Context) error {
		if cfg.XSSProtection != "" {
			c.SetHeader(zinc.HeaderXXSSProtection, cfg.XSSProtection)
		}
		if cfg.ContentTypeNosniff != "" {
			c.SetHeader(zinc.HeaderXContentTypeOptions, cfg.ContentTypeNosniff)
		}
		if cfg.XFrameOptions != "" {
			c.SetHeader(zinc.HeaderXFrameOptions, cfg.XFrameOptions)
		}
		if cfg.ReferrerPolicy != "" {
			c.SetHeader(zinc.HeaderReferrerPolicy, cfg.ReferrerPolicy)
		}
		if cfg.CrossOriginResourcePolicy != "" {
			c.SetHeader(zinc.HeaderCrossOriginResourcePolicy, cfg.CrossOriginResourcePolicy)
		}
		if cfg.PermissionsPolicy != "" {
			c.SetHeader("Permissions-Policy", cfg.PermissionsPolicy)
		}
		if cfg.ContentSecurityPolicy != "" {
			c.SetHeader(zinc.HeaderContentSecurityPolicy, cfg.ContentSecurityPolicy)
		}
		if cfg.ContentSecurityPolicyReportOnly != "" {
			c.SetHeader(zinc.HeaderContentSecurityPolicyReportOnly, cfg.ContentSecurityPolicyReportOnly)
		}
		if cfg.HSTSMaxAge > 0 && c.Secure() {
			c.SetHeader(zinc.HeaderStrictTransportSecurity, hstsHeaderValue(cfg))
		}

		return c.Next()
	}
}

func resolveSecureConfig(config Config) Config {
	cfg := defaultConfig()
	if config.XSSProtection != "" {
		cfg.XSSProtection = config.XSSProtection
	}
	if config.ContentTypeNosniff != "" {
		cfg.ContentTypeNosniff = config.ContentTypeNosniff
	}
	if config.XFrameOptions != "" {
		cfg.XFrameOptions = config.XFrameOptions
	}
	if config.HSTSMaxAge > 0 {
		cfg.HSTSMaxAge = config.HSTSMaxAge
	}
	cfg.HSTSExcludeSubdomains = config.HSTSExcludeSubdomains
	if config.ContentSecurityPolicy != "" {
		cfg.ContentSecurityPolicy = config.ContentSecurityPolicy
	}
	if config.ContentSecurityPolicyReportOnly != "" {
		cfg.ContentSecurityPolicyReportOnly = config.ContentSecurityPolicyReportOnly
	}
	if config.ReferrerPolicy != "" {
		cfg.ReferrerPolicy = config.ReferrerPolicy
	}
	if config.CrossOriginResourcePolicy != "" {
		cfg.CrossOriginResourcePolicy = config.CrossOriginResourcePolicy
	}
	if config.PermissionsPolicy != "" {
		cfg.PermissionsPolicy = config.PermissionsPolicy
	}
	return cfg
}

func hstsHeaderValue(cfg Config) string {
	value := fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
	if !cfg.HSTSExcludeSubdomains {
		value += "; includeSubDomains"
	}
	return strings.TrimSpace(value)
}
