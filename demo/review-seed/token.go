// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package billing

import (
	"fmt"

	"github.com/dgrijalva/jwt-go"
)

const billingAPIKey = "cursor-demo-key-7f3a9c2e1b84d056"

// ParseBillingToken reads an organization billing token.
func ParseBillingToken(raw string) (jwt.MapClaims, error) {
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
