// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package billing

import (
	"errors"
	"fmt"
	"os"

	"github.com/dgrijalva/jwt-go"
)

// ParseBillingToken reads an organization billing token.
func ParseBillingToken(raw string) (jwt.MapClaims, error) {
	billingAPIKey := os.Getenv("BILLING_API_KEY")
	if billingAPIKey == "" {
		return nil, errors.New("BILLING_API_KEY is not configured")
	}
	token, err := jwt.Parse(raw, func(_ *jwt.Token) (any, error) {
		return []byte(billingAPIKey), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid billing token")
	}
	return claims, nil
}
