package security

import (
	"regexp"
	"strings"
)

// PIISanitizer handles PII (Personally Identifiable Information) detection and sanitization
type PIISanitizer struct {
	phonePattern       *regexp.Regexp
	emailPattern       *regexp.Regexp
	idCardPattern     *regexp.Regexp
	sensitivePatterns []string
}

// PIIType represents the type of PII
type PIIType string

const (
	PIIPhone       PIIType = "phone"
	PIIEmail       PIIType = "email"
	PIIIDCard      PIIType = "id_card"
	PIIAddress     PIIType = "address"
	PIISSN          PIIType = "ssn"
	PIIBankAccount PIIType = "bank_account"
	PIIName        PIIType = "name"
)

// SanitizationResult contains the result of PII sanitization
type SanitizationResult struct {
	Original  string
	Sanitized string
	PIITypes []PIIType
	Removed  []SanitizedPII
}

// SanitizedPII represents a piece of PII that was found and removed
type SanitizedPII struct {
	Type  PIIType
	Index int
	Length int
}

// NewPIISanitizer creates a new PII sanitizer
func NewPIISanitizer() *PIISanitizer {
	return &PIISanitizer{
		phonePattern: regexp.MustCompile(`(1[3-9]\d{1,4}[-\s]?\d{3,4}[-\s]?\d{4})`),
		emailPattern:    regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		idCardPattern:  regexp.MustCompile(`\b\d{4}[- ]?\d{4,6}[- ]?\d{4}\b`),
		sensitivePatterns: []string{
			"身份证",
			"银行卡",
			"密码",
			"手机号",
			"邮箱",
			"地址",
			"社保号",
			"护照号",
			"出生日期",
			"家庭住址",
			"工作地址",
			"紧急联系人",
		},
	}
}

// Sanitize removes or masks PII from the input string
func (ps *PIISanitizer) Sanitize(input string) SanitizationResult {
	result := SanitizationResult{
		Original:  input,
		Sanitized: input,
		PIITypes:  make([]PIIType, 0),
		Removed:    make([]SanitizedPII, 0),
	}

	// Check for phone numbers
	phone := ps.phonePattern.FindAllString(input, -1)
	for _, match := range phone {
		masked := ps.maskPhoneNumber(match)
		result.Sanitized = strings.ReplaceAll(result.Sanitized, match, masked)
		result.PIITypes = append(result.PIITypes, PIIPhone)
		result.Removed = append(result.Removed, SanitizedPII{
			Type:  PIIPhone,
			Index: strings.Index(result.Sanitized, masked),
			Length: len(match),
		})
	}

	// Check for email addresses
	email := ps.emailPattern.FindAllString(input, -1)
	for _, match := range email {
		masked := ps.maskEmailAddress(match)
		result.Sanitized = strings.ReplaceAll(result.Sanitized, match, masked)
		result.PIITypes = append(result.PIITypes, PIIEmail)
		result.Removed = append(result.Removed, SanitizedPII{
			Type:  PIIEmail,
			Index: strings.Index(result.Sanitized, masked),
			Length: len(match),
		})
	}

	// Check for ID cards
	idCard := ps.idCardPattern.FindAllString(input, -1)
	for _, match := range idCard {
		masked := ps.maskIDCard(match)
		result.Sanitized = strings.ReplaceAll(result.Sanitized, match, masked)
		result.PIITypes = append(result.PIITypes, PIIIDCard)
		result.Removed = append(result.Removed, SanitizedPII{
			Type:  PIIIDCard,
			Index: strings.Index(result.Sanitized, masked),
			Length: len(match),
		})
	}

	// Check for sensitive patterns (Chinese)
	for _, pattern := range ps.sensitivePatterns {
		if strings.Contains(input, pattern) {
			// Remove the sensitive information
			start := strings.Index(input, pattern)
			if start >= 0 {
				end := start + len(pattern)
				var context string
				if end < len(input) {
					context = input[start:end]
				}
				result.Sanitized = input[:start] + context + input[end:]
				result.PIITypes = append(result.PIITypes, PIIName)
				result.Removed = append(result.Removed, SanitizedPII{
					Type:  PIIName,
					Index: start,
					Length: len(pattern),
				})
			}
		}
	}

	return result
}

// maskPhoneNumber masks a phone number, keeping first 3 and last 4 digits
func (ps *PIISanitizer) maskPhoneNumber(phone string) string {
	// Keep first 3 and last 4 digits, mask middle digits
	if len(phone) >= 7 && len(phone) <= 11 {
		return phone[:3] + "****" + phone[len(phone)-4:]
	}
	return phone
}

// maskEmailAddress masks an email address
func (ps *PIISanitizer) maskEmailAddress(email string) string {
	at := strings.LastIndex(email, "@")
	if at > 0 {
		username := email[:at]
		if len(username) > 2 {
			return username[:2] + "****" + email[at:]
		}
		return string(username[0]) + "*" + email[at:]
	}
	return email
}

// maskIDCard masks an ID card number
func (ps *PIISanitizer) maskIDCard(idCard string) string {
	// Keep first 6 and last 4 digits, mask middle digits
	if len(idCard) >= 10 && len(idCard) <= 18 {
		return idCard[:6] + "********" + idCard[len(idCard)-4:]
	}
	// Simple masking for shorter IDs
	if len(idCard) > 4 {
		return idCard[:2] + "****" + idCard[len(idCard)-2:]
	}
	return idCard
}

// DetectPII detects PII in the input string
func (ps *PIISanitizer) DetectPII(input string) []PIIType {
	found := make([]PIIType, 0)

	// Check for phone numbers
	if ps.phonePattern.MatchString(input) {
		found = append(found, PIIPhone)
	}

	// Check for email addresses
	if ps.emailPattern.MatchString(input) {
		found = append(found, PIIEmail)
	}

	// Check for ID cards
	if ps.idCardPattern.MatchString(input) {
		found = append(found, PIIIDCard)
	}

	// Check for sensitive patterns
	for _, pattern := range ps.sensitivePatterns {
		if strings.Contains(input, pattern) {
			found = append(found, PIIName)
		}
	}

	return found
}

// SanitizePIIForDatabase sanitizes PII for database storage
func (ps *PIISanitizer) SanitizePIIForDatabase(input string) string {
	// For database storage, we can be more aggressive
	// Remove all PII and sensitive information
	result := ps.Sanitize(input)

	// Additional sanitization for database
	for _, pattern := range ps.sensitivePatterns {
		if strings.Contains(result.Sanitized, pattern) {
			result.Sanitized = strings.ReplaceAll(result.Sanitized, pattern, "[REDACTED]")
		}
	}

	return result.Sanitized
}

// HasPII checks if the input contains any PII
func (ps *PIISanitizer) HasPII(input string) bool {
	pii := ps.DetectPII(input)
	return len(pii) > 0
}