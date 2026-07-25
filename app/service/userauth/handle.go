package userauth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	userHandleDisplayWidth            = 5
	userHandleGenerateAttempts        = 10
	userHandleFirstNumber      uint64 = 1
)

func (s *userAuthService) generateOAuthHandle(ctx context.Context, offset uint64) (string, error) {
	maxHandle, err := s.userModel.GetMaxNumericHandle(ctx)
	if err != nil {
		return "", err
	}
	return formatUserHandleNumber(maxHandle + offset + userHandleFirstNumber), nil
}

func formatUserHandleNumber(number uint64) string {
	if number < userHandleFirstNumber {
		return ""
	}
	if number < 100000 {
		return fmt.Sprintf("%0*d", userHandleDisplayWidth, number)
	}
	return strconv.FormatUint(number, 10)
}

func isDuplicateUserHandleError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") && strings.Contains(message, "idx_user_handle")
}
