package config

import (
	"testing"
)

func BenchmarkConfigHash(b *testing.B) {
	dir := b.TempDir()
	_ = EnsureSkeleton(dir)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = configHash(dir)
	}
}

func BenchmarkLoadServices(b *testing.B) {
	dir := b.TempDir()
	_ = EnsureSkeleton(dir)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loadServices(dir)
	}
}

func BenchmarkLoadSettings(b *testing.B) {
	dir := b.TempDir()
	_ = EnsureSkeleton(dir)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loadSettings(dir)
	}
}
