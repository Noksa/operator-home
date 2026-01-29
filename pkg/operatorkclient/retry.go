package operatorkclient

import (
	"time"

	"go.uber.org/multierr"
)

// Retry executes fn up to maxAttempts times, sleeping between attempts
func Retry(maxAttempts int, sleepBetweenAttempts time.Duration, fn func() error) error {
	if sleepBetweenAttempts <= 0 {
		sleepBetweenAttempts = time.Millisecond * 10
	}
	var mErr error
	for i := 1; i <= maxAttempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		mErr = multierr.Append(mErr, err)
		if i < maxAttempts {
			time.Sleep(sleepBetweenAttempts)
		}
	}
	return mErr
}
