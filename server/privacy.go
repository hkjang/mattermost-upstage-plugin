package main

import (
	"regexp"
	"strings"
)

var (
	emailPattern          = regexp.MustCompile(`\b([A-Za-z0-9._%+\-]{1,64})@([A-Za-z0-9.\-]+\.[A-Za-z]{2,})\b`)
	phonePattern          = regexp.MustCompile(`(^|[^0-9])((?:\+?82[- ]?)?0\d{1,2}[- ]?\d{3,4}[- ]?\d{4})([^0-9]|$)`)
	residentIDPattern     = regexp.MustCompile(`(^|[^0-9])(\d{6}[- ]?[1-8]\d{6})([^0-9]|$)`)
	longDigitGroupPattern = regexp.MustCompile(`(^|[^0-9])((?:\d[ -]?){12,18}\d)([^0-9]|$)`)
)

func maskSensitiveContent(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}

	masked := emailPattern.ReplaceAllStringFunc(value, maskEmailAddress)
	masked = maskPatternGroup(masked, phonePattern, func(token string) string {
		return maskDigits(token, 3, 4)
	})
	masked = maskPatternGroup(masked, residentIDPattern, func(token string) string {
		return maskDigits(token, 0, 0)
	})
	masked = maskPatternGroup(masked, longDigitGroupPattern, func(token string) string {
		if countDigits(token) < 13 {
			return token
		}
		return maskDigits(token, 0, 4)
	})

	return masked
}

func maskPatternGroup(value string, pattern *regexp.Regexp, mask func(string) string) string {
	return pattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := pattern.FindStringSubmatch(match)
		if len(groups) < 4 {
			return match
		}
		return groups[1] + mask(groups[2]) + groups[3]
	})
}

func maskEmailAddress(value string) string {
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 {
		return value
	}

	local := parts[0]
	switch {
	case len(local) <= 1:
		local = "*"
	case len(local) == 2:
		local = local[:1] + "*"
	default:
		local = local[:2] + strings.Repeat("*", len(local)-2)
	}

	return local + "@" + parts[1]
}

func maskDigits(value string, keepLeading, keepTrailing int) string {
	totalDigits := countDigits(value)
	if totalDigits == 0 {
		return value
	}

	if keepLeading+keepTrailing >= totalDigits {
		keepLeading = 0
		keepTrailing = 0
	}

	maskAfter := keepLeading
	maskBefore := totalDigits - keepTrailing
	seenDigits := 0
	var builder strings.Builder
	builder.Grow(len(value))

	for _, character := range value {
		if character < '0' || character > '9' {
			builder.WriteRune(character)
			continue
		}

		if seenDigits < maskAfter || seenDigits >= maskBefore {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('*')
		}
		seenDigits++
	}

	return builder.String()
}

func countDigits(value string) int {
	count := 0
	for _, character := range value {
		if character >= '0' && character <= '9' {
			count++
		}
	}
	return count
}
