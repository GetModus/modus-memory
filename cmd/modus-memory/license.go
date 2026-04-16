package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// License tiers
const (
	TierFree = "free"
	TierPro  = "pro"
)

// Free tier limits
const (
	FreeDocLimit = 1000
)

// Lemon Squeezy license validation endpoint
const lemonSqueezyValidateURL = "https://api.lemonsqueezy.com/v1/licenses/validate"
const lemonSqueezyActivateURL = "https://api.lemonsqueezy.com/v1/licenses/activate"
const lemonSqueezyDeactivateURL = "https://api.lemonsqueezy.com/v1/licenses/deactivate"

// How long to cache a successful validation before re-checking
const validationCacheDays = 7

// licenseState is the on-disk cached state after validation.
type licenseState struct {
	LicenseKey  string `json:"license_key"`
	InstanceID  string `json:"instance_id"` // Lemon Squeezy instance ID for this machine
	Valid       bool   `json:"valid"`
	Tier        string `json:"tier"`
	Email       string `json:"customer_email"`
	CustomerID  int64  `json:"customer_id"`
	ProductName string `json:"product_name"`
	VariantName string `json:"variant_name"`
	ValidatedAt string `json:"validated_at"` // RFC3339 — last successful validation
	ExpiresAt   string `json:"expires_at"`   // subscription expiry from Lemon Squeezy
}

// licenseResult is what the binary uses at runtime.
type licenseResult struct {
	tier   string
	valid  bool
	reason string
	state  *licenseState
}

// lemonSqueezyResponse is the API response from validate/activate.
type lemonSqueezyResponse struct {
	Valid    bool   `json:"valid"`
	Error    string `json:"error"`
	Meta     struct {
		StoreID       int    `json:"store_id"`
		OrderID       int    `json:"order_id"`
		CustomerID    int    `json:"customer_id"`
		CustomerName  string `json:"customer_name"`
		CustomerEmail string `json:"customer_email"`
		ProductID     int    `json:"product_id"`
		ProductName   string `json:"product_name"`
		VariantID     int    `json:"variant_id"`
		VariantName   string `json:"variant_name"`
	} `json:"meta"`
	LicenseKey struct {
		Status    string `json:"status"`    // "active", "inactive", "expired", "disabled"
		ExpiresAt string `json:"expires_at"` // nullable
	} `json:"license_key"`
	Instance struct {
		ID string `json:"id"`
	} `json:"instance"`
}

func licenseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".modus-memory")
}

func licenseStatePath() string {
	return filepath.Join(licenseDir(), "license.json")
}

// loadLicense reads the cached license state and checks if it's still valid.
// Only hits the network if the cache is stale (older than validationCacheDays).
func loadLicense() *licenseResult {
	data, err := os.ReadFile(licenseStatePath())
	if err != nil {
		return &licenseResult{tier: TierFree, valid: false, reason: "no license"}
	}

	var state licenseState
	if err := json.Unmarshal(data, &state); err != nil {
		return &licenseResult{tier: TierFree, valid: false, reason: "corrupt license file"}
	}

	if !state.Valid {
		return &licenseResult{tier: TierFree, valid: false, reason: "license not valid", state: &state}
	}

	// Check if subscription has expired (from Lemon Squeezy's expires_at)
	if state.ExpiresAt != "" {
		if expiry, err := time.Parse(time.RFC3339, state.ExpiresAt); err == nil {
			if time.Now().After(expiry) {
				return &licenseResult{tier: TierFree, valid: false, reason: "subscription expired", state: &state}
			}
		}
	}

	// Check if cached validation is stale — if so, try to re-validate silently
	if state.ValidatedAt != "" {
		if validated, err := time.Parse(time.RFC3339, state.ValidatedAt); err == nil {
			if time.Since(validated) > time.Duration(validationCacheDays)*24*time.Hour {
				// Try to re-validate in the background — if it fails, use cached state
				// (allows offline usage for up to validationCacheDays)
				if refreshed := validateOnline(state.LicenseKey, state.InstanceID); refreshed != nil {
					return refreshed
				}
			}
		}
	}

	return &licenseResult{tier: state.Tier, valid: true, state: &state}
}

// validateOnline checks with Lemon Squeezy. Returns nil if network fails (use cache).
func validateOnline(licenseKey, instanceID string) *licenseResult {
	form := url.Values{
		"license_key": {licenseKey},
	}
	if instanceID != "" {
		form.Set("instance_id", instanceID)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(lemonSqueezyValidateURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil // Network error — use cache
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var lsResp lemonSqueezyResponse
	if err := json.Unmarshal(body, &lsResp); err != nil {
		return nil
	}

	state := &licenseState{
		LicenseKey:  licenseKey,
		InstanceID:  instanceID,
		Valid:       lsResp.Valid && lsResp.LicenseKey.Status == "active",
		Tier:        TierPro,
		Email:       lsResp.Meta.CustomerEmail,
		CustomerID:  int64(lsResp.Meta.CustomerID),
		ProductName: lsResp.Meta.ProductName,
		VariantName: lsResp.Meta.VariantName,
		ValidatedAt: time.Now().Format(time.RFC3339),
		ExpiresAt:   lsResp.LicenseKey.ExpiresAt,
	}

	if !state.Valid {
		state.Tier = TierFree
	}

	// Save updated state
	saveLicenseState(state)

	if !state.Valid {
		reason := "subscription inactive"
		if lsResp.Error != "" {
			reason = lsResp.Error
		}
		return &licenseResult{tier: TierFree, valid: false, reason: reason, state: state}
	}

	return &licenseResult{tier: TierPro, valid: true, state: state}
}

// activateLicense validates a key with Lemon Squeezy, activates an instance, and saves state.
func activateLicense(key string) error {
	// Activate with Lemon Squeezy (registers this machine as an instance)
	hostname, _ := os.Hostname()
	instanceName := fmt.Sprintf("modus-memory-%s", hostname)

	form := url.Values{
		"license_key":   {key},
		"instance_name": {instanceName},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(lemonSqueezyActivateURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("cannot reach license server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var lsResp lemonSqueezyResponse
	if err := json.Unmarshal(body, &lsResp); err != nil {
		return fmt.Errorf("invalid response from license server")
	}

	if !lsResp.Valid {
		msg := lsResp.Error
		if msg == "" {
			msg = "key not recognized"
		}
		return fmt.Errorf("activation failed: %s", msg)
	}

	if lsResp.LicenseKey.Status != "active" {
		return fmt.Errorf("license status: %s (expected active)", lsResp.LicenseKey.Status)
	}

	state := &licenseState{
		LicenseKey:  key,
		InstanceID:  lsResp.Instance.ID,
		Valid:       true,
		Tier:        TierPro,
		Email:       lsResp.Meta.CustomerEmail,
		CustomerID:  int64(lsResp.Meta.CustomerID),
		ProductName: lsResp.Meta.ProductName,
		VariantName: lsResp.Meta.VariantName,
		ValidatedAt: time.Now().Format(time.RFC3339),
		ExpiresAt:   lsResp.LicenseKey.ExpiresAt,
	}

	if err := saveLicenseState(state); err != nil {
		return fmt.Errorf("save license: %w", err)
	}

	fmt.Println("License activated.")
	fmt.Printf("  Tier: Pro\n")
	fmt.Printf("  Email: %s\n", state.Email)
	fmt.Printf("  Product: %s\n", state.ProductName)
	if state.ExpiresAt != "" {
		fmt.Printf("  Renews: %s\n", state.ExpiresAt)
	}

	return nil
}

// refreshLicense re-validates the existing license with Lemon Squeezy.
func refreshLicense() error {
	data, err := os.ReadFile(licenseStatePath())
	if err != nil {
		return fmt.Errorf("no license to refresh — run: modus-memory activate <key>")
	}

	var state licenseState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("corrupt license file")
	}

	result := validateOnline(state.LicenseKey, state.InstanceID)
	if result == nil {
		return fmt.Errorf("cannot reach license server — try again later")
	}

	if result.valid {
		fmt.Println("License valid.")
		fmt.Printf("  Tier: Pro\n")
		fmt.Printf("  Email: %s\n", result.state.Email)
		if result.state.ExpiresAt != "" {
			fmt.Printf("  Renews: %s\n", result.state.ExpiresAt)
		}
	} else {
		fmt.Printf("License no longer valid: %s\n", result.reason)
		fmt.Println("Reverted to free tier.")
		fmt.Println("\nRenew at: https://modus-memory.lemonsqueezy.com")
	}

	return nil
}

// deactivateLicense deactivates the instance with Lemon Squeezy and removes local state.
func deactivateLicense() error {
	data, err := os.ReadFile(licenseStatePath())
	if err != nil {
		fmt.Println("No active license.")
		return nil
	}

	var state licenseState
	if err := json.Unmarshal(data, &state); err == nil && state.LicenseKey != "" && state.InstanceID != "" {
		// Tell Lemon Squeezy to deactivate this instance
		form := url.Values{
			"license_key": {state.LicenseKey},
			"instance_id": {state.InstanceID},
		}
		client := &http.Client{Timeout: 10 * time.Second}
		client.Post(lemonSqueezyDeactivateURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		// Best effort — don't fail if network is down
	}

	os.Remove(licenseStatePath())
	fmt.Println("License deactivated. Reverted to free tier.")
	return nil
}

func saveLicenseState(state *licenseState) error {
	dir := licenseDir()
	os.MkdirAll(dir, 0700)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(licenseStatePath(), data, 0600)
}
