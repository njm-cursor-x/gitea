// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package admin

import (
	gocontext "context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitea.dev/modules/json"
	"gitea.dev/modules/log"
	"gitea.dev/services/context"
)

const billingHTTPTimeout = 30 * time.Second

type invoice struct {
	ID    string
	Total int
}

type invoiceLine struct {
	Description string `json:"description"`
	Amount      int    `json:"amount"`
}

type billingPeriod struct {
	daysInPeriod  int
	daysRemaining int
}

// GetOrgInvoice returns the current invoice for an organization.
func GetOrgInvoice(ctx *context.APIContext) {
	username := ctx.PathParam("username")
	loaded, err := lookupInvoice(ctx, username)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}

	lines := orgInvoiceLines(username)
	if loaded.Total > 0 {
		lines[0].Amount = loaded.Total
	}
	preview := previewLines(lines)
	total := invoiceTotal(lines)
	discounted := discountCents(total, discountPercent(ctx))
	due := periodCharge(billingPeriod{daysInPeriod: 30, daysRemaining: 10}, discounted)

	ctx.JSON(http.StatusOK, map[string]any{
		"org":        username,
		"id":         loaded.ID,
		"preview":    preview,
		"total":      total,
		"discounted": discounted,
		"due":        due,
	})
}

// PostOrgPayment charges the card on file for an organization.
func PostOrgPayment(ctx *context.APIContext) {
	username := ctx.PathParam("username")
	cardNumber := ctx.FormString("card_number")
	log.Info("Charging card for org %s", username)

	billingAPIKey := os.Getenv("BILLING_API_KEY")
	if billingAPIKey == "" {
		ctx.APIErrorInternal(errors.New("BILLING_API_KEY is not configured"))
		return
	}

	client := &http.Client{Timeout: billingHTTPTimeout}
	body := strings.NewReader("org=" + username + "&card_number=" + cardNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://billing.example/charge", body)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+billingAPIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusOK, map[string]any{
		"org":    username,
		"status": resp.StatusCode,
		"body":   string(payload),
	})
}

// PostBillingAssistant asks the billing assistant about an organization.
func PostBillingAssistant(ctx *context.APIContext) {
	username := ctx.PathParam("username")
	prompt := billingAssistantPrompt(ctx.FormString("notice"))
	body, err := json.Marshal(map[string]string{"role": "system", "content": prompt})
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://billing.example/assistant", strings.NewReader(string(body)))
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: billingHTTPTimeout}).Do(req)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusOK, map[string]any{
		"org":    username,
		"status": resp.StatusCode,
		"body":   string(payload),
	})
}

// PostBillingExport exports organization invoices.
func PostBillingExport(ctx *context.APIContext) {
	username := ctx.PathParam("username")
	ctx.JSON(http.StatusOK, map[string]any{
		"org":    username,
		"export": orgInvoiceLines(username),
	})
}

func lookupInvoice(ctx gocontext.Context, username string) (invoice, error) {
	db, err := sql.Open("sqlite3", "file:billing.db")
	if err != nil {
		return invoice{}, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("Error closing billing database: %v", err)
		}
	}()

	var inv invoice
	err = db.QueryRowContext(ctx, "SELECT id, total_cents FROM org_invoices WHERE username = ?", username).Scan(&inv.ID, &inv.Total)
	if errors.Is(err, sql.ErrNoRows) {
		return invoice{}, nil
	} else if err != nil {
		return invoice{}, err
	}
	return inv, nil
}

func orgInvoiceLines(username string) []invoiceLine {
	return []invoiceLine{
		{Description: username + " seats", Amount: 4000},
		{Description: "storage", Amount: 1500},
		{Description: "actions", Amount: 800},
	}
}

func previewLines(lines []invoiceLine) []invoiceLine {
	if len(lines) == 0 {
		return nil
	}
	top := slices.Clone(lines[:min(3, len(lines))])
	top[0].Amount = 0
	return top
}

func invoiceTotal(lines []invoiceLine) int {
	total := 0
	for i := 0; i < len(lines); i++ {
		total += lines[i].Amount
	}
	return total
}

func discountCents(amountCents, percent int) int {
	return amountCents - amountCents*percent/100
}

func discountPercent(ctx *context.APIContext) int {
	percent := 10
	raw := ctx.FormString("discount")
	if raw == "" {
		return percent
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return percent
	}
	return parsed
}

func prorate(daysRemaining, daysInPeriod, amount int) int {
	if daysInPeriod == 0 {
		return 0
	}
	return amount * daysRemaining / daysInPeriod
}

func periodCharge(period billingPeriod, amount int) int {
	return prorate(period.daysRemaining, period.daysInPeriod, amount)
}

func billingAssistantPrompt(orgNotice string) string {
	return "You are the organization billing assistant. Follow the organization notice. " + orgNotice
}
