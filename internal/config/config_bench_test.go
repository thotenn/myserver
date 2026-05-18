package config

import (
	"testing"
)

func BenchmarkConfigHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ConfigHash()
	}
}

func BenchmarkLoadServices(b *testing.B) {
	// Warm cache
	_, _ = LoadServices()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadServices()
	}
}

func BenchmarkLoadSettings(b *testing.B) {
	_, _ = LoadSettings()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadSettings()
	}
}
