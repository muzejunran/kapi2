package security

import (
	"regexp"
	"strings"
	"unicode"
)

// PromptInjector protects against prompt injection attacks
type PromptInjector struct {
	injectionPatterns []*regexp.Regexp
	forbiddenPhrases  []string
	allowedCommands  []string
	maxInputLength   int
	maxOutputLength  int
}

// InjectionType represents the type of prompt injection
type InjectionType string

const (
	InjectionSystemPrompt   InjectionType = "system_prompt"
	InjectionRolePlay        InjectionType = "role_play"
	InjectionOutputOverride InjectionType = "output_override"
	InjectionTokenManipulation InjectionType = "token_manipulation"
	InjectionContextOverflow   InjectionType = "context_overflow"
	InjectionJailbreak       InjectionType = "jailbreak"
)

// InjectionResult contains the result of injection detection
type InjectionResult struct {
	IsInjection bool
	Type        InjectionType
	Confidence  float64
	Matched     string
	Sanitized   string
}

// NewPromptInjector creates a new prompt injection detector
func NewPromptInjector() *PromptInjector {
	// Common injection patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(ignore|disregard|forget).*?(previous|above|all).*(instructions|rules|commands|prompts)`),
		regexp.MustCompile(`(?i)(act|behave|pretend|roleplay).*?(as|like).*?(assistant|ai|model).*?(that|which).*?(ignores|disregards|bypasses)`),
		regexp.MustCompile(`(?i)(system|developer).*?(message|prompt|instruction).*?(is|says).*?`),
		regexp.MustCompile(`(?i)(output|print|display).*?(only|exactly).*?(the).*?(output|response)`),
		regexp.MustCompile(`(?i)(new|different|alternate).*?(role|persona|character)`),
		regexp.MustCompile(`(?i)(beginning|start).*?(with).*?(the).*?(following)`),
		regexp.MustCompile(`(?i)(step|part|section).*?\d+.*?of.*?(\d+|the).*?(process|instruction)`),
		regexp.MustCompile(`(?i)(under no circumstances|in any case|no matter what).*?(should|must).*?(not|never)`),
		regexp.MustCompile(`(?i)(classif(y|ied|ication).*?(as|is).*?(not|never).*?(relevant|important)`),
		regexp.MustCompile(`(?i)(translate|convert).*?(the).*?(above|following).*?(into)`),
	}

	forbiddenPhrases := []string{
		"ignore all previous instructions",
		"disregard system message",
		"forget everything above",
		"you are now",
		"from now on",
		"act as",
		"pretend to be",
		"roleplay as",
		"developer mode",
		"jailbreak",
		"bypass filters",
		"ignore safety",
		"unrestricted mode",
		"no filters",
		"no restrictions",
		"override protocol",
		"new persona",
		"alternate reality",
		"hypothetical scenario",
		"fictional context",
		"simulated environment",
		"assume the role",
		"take on the persona",
		"in this fictional scenario",
		"imagine that",
		"as an AI",
		"bypass the following",
		"your instructions are now",
		"your new task is",
		"restart with new instructions",
		"delete the previous",
		"replace the context",
		"clear the memory",
	}

	allowedCommands := []string{
		"analyze",
		"explain",
		"summarize",
		"translate",
		"generate",
		"calculate",
		"compare",
		"recommend",
		"help",
		"show",
		"list",
		"find",
		"search",
	}

	return &PromptInjector{
		injectionPatterns: patterns,
		forbiddenPhrases:  forbiddenPhrases,
		allowedCommands:  allowedCommands,
		maxInputLength:  10000,  // 10K characters
		maxOutputLength: 5000,  // 5K characters
	}
}

// DetectInjection detects if the input contains prompt injection
func (pi *PromptInjector) DetectInjection(input string) InjectionResult {
	result := InjectionResult{
		IsInjection: false,
		Confidence:  0.0,
	}

	// Check input length
	if len(input) > pi.maxInputLength {
		result.IsInjection = true
		result.Type = InjectionContextOverflow
		result.Confidence = 0.9
		result.Matched = "Input exceeds maximum length"
		return result
	}

	// Check for injection patterns
	for _, pattern := range pi.injectionPatterns {
		if pattern.MatchString(input) {
			result.IsInjection = true
			result.Confidence = 0.85
			result.Matched = pattern.String()
			result.Type = pi.classifyInjection(result.Matched)
			return result
		}
	}

	// Check for forbidden phrases
	lowerInput := strings.ToLower(input)
	for _, phrase := range pi.forbiddenPhrases {
		if strings.Contains(lowerInput, strings.ToLower(phrase)) {
			result.IsInjection = true
			result.Confidence = 0.75
			result.Matched = phrase
			result.Type = pi.classifyInjection(phrase)
			return result
		}
	}

	// Check for role-play attempts
	if pi.detectRolePlay(input) {
		result.IsInjection = true
		result.Type = InjectionRolePlay
		result.Confidence = 0.7
		result.Matched = "Role-play attempt detected"
		return result
	}

	// Check for output override attempts
	if pi.detectOutputOverride(input) {
		result.IsInjection = true
		result.Type = InjectionOutputOverride
		result.Confidence = 0.8
		result.Matched = "Output override attempt detected"
		return result
	}

	return result
}

// SanitizeInput sanitizes potentially malicious input
func (pi *PromptInjector) SanitizeInput(input string) string {
	sanitized := input

	// Remove injection patterns
	for _, pattern := range pi.injectionPatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "[INJECTION_BLOCKED]")
	}

	// Remove forbidden phrases
	for _, phrase := range pi.forbiddenPhrases {
		sanitized = strings.ReplaceAll(sanitized, phrase, "[PHRASE_BLOCKED]")
	}

	// Remove excessive special characters
	sanitized = pi.cleanSpecialChars(sanitized)

	// Remove repeated characters
	sanitized = pi.removeRepeatedChars(sanitized)

	return sanitized
}

// classifyInjection classifies the type of injection based on the matched pattern
func (pi *PromptInjector) classifyInjection(matched string) InjectionType {
	matchedLower := strings.ToLower(matched)

	if strings.Contains(matchedLower, "ignore") ||
	   strings.Contains(matchedLower, "disregard") ||
	   strings.Contains(matchedLower, "forget") {
		return InjectionSystemPrompt
	}

	if strings.Contains(matchedLower, "act") ||
	   strings.Contains(matchedLower, "behave") ||
	   strings.Contains(matchedLower, "pretend") ||
	   strings.Contains(matchedLower, "roleplay") {
		return InjectionRolePlay
	}

	if strings.Contains(matchedLower, "output") ||
	   strings.Contains(matchedLower, "print") ||
	   strings.Contains(matchedLower, "display") {
		return InjectionOutputOverride
	}

	if strings.Contains(matchedLower, "token") {
		return InjectionTokenManipulation
	}

	return InjectionJailbreak
}

// detectRolePlay detects role-play attempts
func (pi *PromptInjector) detectRolePlay(input string) bool {
	rolePlayIndicators := []string{
		"act as",
		"behave as",
		"pretend to be",
		"roleplay as",
		"take on the persona",
		"assume the role",
		"you are now",
		"from now on",
	}

	lowerInput := strings.ToLower(input)
	for _, indicator := range rolePlayIndicators {
		if strings.Contains(lowerInput, indicator) {
			return true
		}
	}

	return false
}

// detectOutputOverride detects attempts to override output
func (pi *PromptInjector) detectOutputOverride(input string) bool {
	overrideIndicators := []string{
		"only output",
		"exactly output",
		"print only",
		"display only",
		"output exactly",
		"return only",
	}

	lowerInput := strings.ToLower(input)
	for _, indicator := range overrideIndicators {
		if strings.Contains(lowerInput, indicator) {
			return true
		}
	}

	return false
}

// cleanSpecialChars removes excessive special characters
func (pi *PromptInjector) cleanSpecialChars(input string) string {
	var cleaned strings.Builder
	specialCount := 0

	for _, r := range input {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			specialCount++
			if specialCount > 3 { // More than 3 consecutive special chars
				continue
			}
		} else {
			specialCount = 0
		}
		cleaned.WriteRune(r)
	}

	return cleaned.String()
}

// removeRepeatedChars removes repeated characters (e.g., "aaaa" -> "a")
func (pi *PromptInjector) removeRepeatedChars(input string) string {
	if len(input) == 0 {
		return input
	}

	var cleaned strings.Builder
	previous := rune(0)
	repeatCount := 0
	maxRepeats := 3

	for _, r := range input {
		if r == previous {
			repeatCount++
			if repeatCount <= maxRepeats {
				cleaned.WriteRune(r)
			}
		} else {
			repeatCount = 1
			cleaned.WriteRune(r)
		}
		previous = r
	}

	return cleaned.String()
}

// ValidateOutput validates the model output against security constraints
func (pi *PromptInjector) ValidateOutput(output string) []string {
	violations := make([]string, 0)

	// Check output length
	if len(output) > pi.maxOutputLength {
		violations = append(violations, "Output exceeds maximum length")
	}

	// Check for leaked system instructions
	if pi.containsSystemInstructions(output) {
		violations = append(violations, "Output contains system instructions")
	}

	// Check for inappropriate content
	if pi.containsInappropriateContent(output) {
		violations = append(violations, "Output contains inappropriate content")
	}

	return violations
}

// containsSystemInstructions checks if output contains leaked system instructions
func (pi *PromptInjector) containsSystemInstructions(output string) bool {
	systemKeywords := []string{
		"system prompt",
		"system instruction",
		"as an AI assistant",
		"my instructions",
		"my programming",
		"my guidelines",
	}

	lowerOutput := strings.ToLower(output)
	for _, keyword := range systemKeywords {
		if strings.Contains(lowerOutput, keyword) {
			return true
		}
	}

	return false
}

// containsInappropriateContent checks for inappropriate content
func (pi *PromptInjector) containsInappropriateContent(output string) bool {
	// Simple check - in production, use more sophisticated content filtering
	inappropriatePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(hack|crack|bypass).*?(password|account|system)`),
		regexp.MustCompile(`(?i)(how to).*?(illegal|banned|prohibited)`),
	}

	for _, pattern := range inappropriatePatterns {
		if pattern.MatchString(output) {
			return true
		}
	}

	return false
}

// SetMaxInputLength sets the maximum input length
func (pi *PromptInjector) SetMaxInputLength(length int) {
	pi.maxInputLength = length
}

// SetMaxOutputLength sets the maximum output length
func (pi *PromptInjector) SetMaxOutputLength(length int) {
	pi.maxOutputLength = length
}

// GetAllowedCommands returns the list of allowed commands
func (pi *PromptInjector) GetAllowedCommands() []string {
	return pi.allowedCommands
}

// AddForbiddenPhrase adds a new forbidden phrase
func (pi *PromptInjector) AddForbiddenPhrase(phrase string) {
	pi.forbiddenPhrases = append(pi.forbiddenPhrases, phrase)
}

// RemoveForbiddenPhrase removes a forbidden phrase
func (pi *PromptInjector) RemoveForbiddenPhrase(phrase string) {
	for i, fp := range pi.forbiddenPhrases {
		if strings.EqualFold(fp, phrase) {
			pi.forbiddenPhrases = append(pi.forbiddenPhrases[:i], pi.forbiddenPhrases[i+1:]...)
			break
		}
	}
}