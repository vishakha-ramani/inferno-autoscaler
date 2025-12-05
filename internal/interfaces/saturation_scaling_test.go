package interfaces

import (
	"testing"
)

func TestDefaultSaturationScalingConfig(t *testing.T) {
	config := DefaultSaturationScalingConfig()

	if config.KvCacheThreshold != 0.80 {
		t.Errorf("Expected KvCacheThreshold 0.80, got %.2f", config.KvCacheThreshold)
	}
	if config.QueueLengthThreshold != 5 {
		t.Errorf("Expected QueueLengthThreshold 5, got %.1f", config.QueueLengthThreshold)
	}
	if config.KvSpareTrigger != 0.10 {
		t.Errorf("Expected KvSpareTrigger 0.10, got %.2f", config.KvSpareTrigger)
	}
	if config.QueueSpareTrigger != 3 {
		t.Errorf("Expected QueueSpareTrigger 3, got %.1f", config.QueueSpareTrigger)
	}
}

func TestSaturationScalingConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  SaturationScalingConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultSaturationScalingConfig(),
			wantErr: false,
		},
		{
			name: "valid custom config",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.75,
				QueueLengthThreshold: 10,
				KvSpareTrigger:       0.15,
				QueueSpareTrigger:    5,
			},
			wantErr: false,
		},
		{
			name: "invalid KvCacheThreshold too high",
			config: SaturationScalingConfig{
				KvCacheThreshold:     1.5,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       0.1,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "invalid KvCacheThreshold negative",
			config: SaturationScalingConfig{
				KvCacheThreshold:     -0.1,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       0.1,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "invalid QueueLengthThreshold negative",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.8,
				QueueLengthThreshold: -1,
				KvSpareTrigger:       0.1,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "invalid KvSpareTrigger too high",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.8,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       1.5,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "invalid KvSpareTrigger negative",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.8,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       -0.1,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "invalid QueueSpareTrigger negative",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.8,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       0.1,
				QueueSpareTrigger:    -1,
			},
			wantErr: true,
		},
		{
			name: "invalid KvCacheThreshold less than KvSpareTrigger",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.5,
				QueueLengthThreshold: 5,
				KvSpareTrigger:       0.6,
				QueueSpareTrigger:    3,
			},
			wantErr: true,
		},
		{
			name: "edge case: zero values are valid",
			config: SaturationScalingConfig{
				KvCacheThreshold:     0.0,
				QueueLengthThreshold: 0,
				KvSpareTrigger:       0.0,
				QueueSpareTrigger:    0,
			},
			wantErr: false,
		},
		{
			name: "edge case: max values are valid",
			config: SaturationScalingConfig{
				KvCacheThreshold:     1.0,
				QueueLengthThreshold: 1000,
				KvSpareTrigger:       1.0,
				QueueSpareTrigger:    1000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaturationScalingConfigMerge(t *testing.T) {
	tests := []struct {
		name     string
		base     SaturationScalingConfig
		override SaturationScalingConfig
		expected SaturationScalingConfig
	}{
		{
			name: "full override",
			base: DefaultSaturationScalingConfig(),
			override: SaturationScalingConfig{
				KvCacheThreshold:     0.75,
				QueueLengthThreshold: 10,
				KvSpareTrigger:       0.15,
				QueueSpareTrigger:    5,
			},
			expected: SaturationScalingConfig{
				KvCacheThreshold:     0.75,
				QueueLengthThreshold: 10,
				KvSpareTrigger:       0.15,
				QueueSpareTrigger:    5,
			},
		},
		{
			name: "partial override - only KvCacheThreshold",
			base: DefaultSaturationScalingConfig(),
			override: SaturationScalingConfig{
				KvCacheThreshold: 0.90,
			},
			expected: SaturationScalingConfig{
				KvCacheThreshold:     0.90,
				QueueLengthThreshold: 5,   // from default
				KvSpareTrigger:       0.1, // from default
				QueueSpareTrigger:    3,   // from default
			},
		},
		{
			name: "partial override - queue thresholds only",
			base: DefaultSaturationScalingConfig(),
			override: SaturationScalingConfig{
				QueueLengthThreshold: 20,
				QueueSpareTrigger:    10,
			},
			expected: SaturationScalingConfig{
				KvCacheThreshold:     0.8, // from default
				QueueLengthThreshold: 20,
				KvSpareTrigger:       0.1, // from default
				QueueSpareTrigger:    10,
			},
		},
		{
			name:     "empty override - base unchanged",
			base:     DefaultSaturationScalingConfig(),
			override: SaturationScalingConfig{},
			expected: DefaultSaturationScalingConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.base
			result.Merge(tt.override)

			if result.KvCacheThreshold != tt.expected.KvCacheThreshold {
				t.Errorf("KvCacheThreshold = %.2f, want %.2f", result.KvCacheThreshold, tt.expected.KvCacheThreshold)
			}
			if result.QueueLengthThreshold != tt.expected.QueueLengthThreshold {
				t.Errorf("QueueLengthThreshold = %.1f, want %.1f", result.QueueLengthThreshold, tt.expected.QueueLengthThreshold)
			}
			if result.KvSpareTrigger != tt.expected.KvSpareTrigger {
				t.Errorf("KvSpareTrigger = %.2f, want %.2f", result.KvSpareTrigger, tt.expected.KvSpareTrigger)
			}
			if result.QueueSpareTrigger != tt.expected.QueueSpareTrigger {
				t.Errorf("QueueSpareTrigger = %.1f, want %.1f", result.QueueSpareTrigger, tt.expected.QueueSpareTrigger)
			}
		})
	}
}
