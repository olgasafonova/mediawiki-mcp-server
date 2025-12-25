package cache

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkCacheGet(b *testing.B) {
	c := New(1 * time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("key-%d", i%1000))
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := New(1 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}
}

func BenchmarkCacheGetSet(b *testing.B) {
	c := New(1 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%100)
		if _, ok := c.Get(key); !ok {
			c.Set(key, fmt.Sprintf("value-%d", i))
		}
	}
}

func BenchmarkCacheParallel(b *testing.B) {
	c := New(1 * time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				c.Get(key)
			} else {
				c.Set(key, fmt.Sprintf("value-%d", i))
			}
			i++
		}
	})
}
