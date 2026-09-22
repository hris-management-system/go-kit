package lib

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rvldodo/hris-kit/constant"
)

func GetStringPointerStatus(s string) *string {
	switch s {
	case "0", "", "Null":
		unclean := "Unclean Data (No Call)"
		return &unclean
	default:
		return &s
	}
}

// DerefString reads a nullable column into a plain string. users.email and
// users.phone_number are both nullable — a signup fills in one and leaves the
// other NULL — so dereferencing either without a nil check panics the request.
func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func GetStringPointer(s string) *string {
	switch s {
	case "0", "", "Null":
		return nil
	default:
		return &s
	}
}

func ParseFloatPointer(s string) *float64 {
	if s == "" {
		return nil
	}
	str := strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return nil
	}
	return &val
}

func ParseFloat(s string) float64 {
	if s == "" {
		return 0
	}

	str := strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0
	}
	return val
}

func ParseInt(s string) int {
	if s == "" {
		return 0
	}

	str := strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return val
}

func ParseDateToDateTime2(dateStr string) (time.Time, error) {
	// Trim whitespace
	dateStr = strings.TrimSpace(dateStr)

	// Try multiple layouts
	layouts := []string{
		"01/02/2006", // MM/DD/YYYY (e.g., "01/15/2026")
		"01/02/06",   // MM/DD/YY   (e.g., "01/15/26")
	}

	var parsedDate time.Time
	var parseErr error

	// Try each layout until one works
	for _, layout := range layouts {
		parsedDate, parseErr = time.Parse(layout, dateStr)
		if parseErr == nil {
			// Successfully parsed
			break
		}
	}

	// If all layouts failed
	if parseErr != nil {
		return time.Time{}, fmt.Errorf("failed to parse date '%s': %w", dateStr, parseErr)
	}

	// Set time to current time (like SYSDATETIME())
	now := time.Now()

	// Combine parsed date with current time
	dateTime := time.Date(
		parsedDate.Year(),
		parsedDate.Month(),
		parsedDate.Day(),
		now.Hour(),
		now.Minute(),
		now.Second(),
		now.Nanosecond(),
		time.Local,
	)

	return dateTime, nil
}

func IsTruthy(value string) bool {
	switch value {
	case "true", "t", "yes", "y", "1", "on":
		return true
	default:
		return false
	}
}

func IsTruthyBit(value string) int {
	switch value {
	case "true", "t", "yes", "y", "1", "on":
		return 1
	default:
		return 0
	}
}

func NormalizePaging(page, pageSize int) (limit, offset int) {
	if page < 1 {
		page = constant.DEFAULT_PAGE
	}
	if pageSize < 1 {
		pageSize = constant.DEFAULT_PAGE_SIZE
	}
	if pageSize > constant.MAX_PAGE_SIZE {
		pageSize = constant.MAX_PAGE_SIZE
	}
	return pageSize, (page - 1) * pageSize
}
