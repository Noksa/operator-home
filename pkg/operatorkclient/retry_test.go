package operatorkclient_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

var _ = Describe("Retry", func() {
	It("should return nil on first success", func() {
		calls := 0
		err := kc.Retry(3, time.Millisecond, func() error {
			calls++
			return nil
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(calls).To(Equal(1))
	})

	It("should retry up to maxAttempts on failure", func() {
		calls := 0
		err := kc.Retry(3, time.Millisecond, func() error {
			calls++
			return fmt.Errorf("fail %d", calls)
		})
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(3))
		// multierr combines all errors
		Expect(err.Error()).To(ContainSubstring("fail 1"))
		Expect(err.Error()).To(ContainSubstring("fail 2"))
		Expect(err.Error()).To(ContainSubstring("fail 3"))
	})

	It("should succeed on a later attempt", func() {
		calls := 0
		err := kc.Retry(5, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return fmt.Errorf("not yet")
			}
			return nil
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(calls).To(Equal(3))
	})

	It("should handle maxAttempts of 1", func() {
		calls := 0
		err := kc.Retry(1, time.Millisecond, func() error {
			calls++
			return fmt.Errorf("only once")
		})
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(1))
	})

	It("should handle maxAttempts of 0 (no attempts)", func() {
		calls := 0
		err := kc.Retry(0, time.Millisecond, func() error {
			calls++
			return fmt.Errorf("should not run")
		})
		// 0 attempts means the loop body never executes
		Expect(err).ToNot(HaveOccurred())
		Expect(calls).To(Equal(0))
	})

	It("should default sleep to 10ms when given zero duration", func() {
		calls := 0
		start := time.Now()
		_ = kc.Retry(3, 0, func() error {
			calls++
			return fmt.Errorf("fail")
		})
		elapsed := time.Since(start)
		Expect(calls).To(Equal(3))
		// With 2 sleeps of 10ms each, should take at least ~15ms
		Expect(elapsed).To(BeNumerically(">=", 15*time.Millisecond))
	})

	It("should default sleep to 10ms when given negative duration", func() {
		calls := 0
		start := time.Now()
		_ = kc.Retry(2, -time.Second, func() error {
			calls++
			return fmt.Errorf("fail")
		})
		elapsed := time.Since(start)
		Expect(calls).To(Equal(2))
		// 1 sleep of 10ms
		Expect(elapsed).To(BeNumerically(">=", 8*time.Millisecond))
	})

	It("should not sleep after the last failed attempt", func() {
		start := time.Now()
		_ = kc.Retry(1, time.Second, func() error {
			return fmt.Errorf("fail")
		})
		elapsed := time.Since(start)
		// With only 1 attempt, there should be no sleep at all
		Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
	})
})
