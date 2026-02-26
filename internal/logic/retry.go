package logic

import (
	"context"
	"time"

	"eldorado-bot/internal/logger"
)

// retryWithBackoff verilen fonksiyonu, hatalarda exponential backoff ile tekrar dener.
func retryWithBackoff(ctx context.Context, log *logger.Logger, attempts int, baseDelay time.Duration, fn func(context.Context) error) error {
	delay := baseDelay
	var lastErr error

	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := fn(ctx); err != nil {
			lastErr = err
			if i == attempts-1 {
				break
			}
			log.Errorf("operation failed (attempt %d/%d): %v; retrying in %s", i+1, attempts, err, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			continue
		}

		return nil
	}

	return lastErr
}

