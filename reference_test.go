package accounting

import "testing"

func TestGenerateEstonianReference(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		want    string
		wantErr bool
	}{
		{
			name: "simple example",
			base: "1234",
			want: "12344", // 1*7 + 2*1 + 3*3 + 4*7 = 46, check = (10 - 6) = 4
		},
		{
			name: "known example from Estonian Banking Association",
			base: "12131295",
			want: "121312952", // Sum = 118, check = (10 - 8) = 2
		},
		{
			name: "another example",
			base: "123456",
			want: "1234561", // Sum = 79, check = (10 - 9) = 1
		},
		{
			name: "single digit",
			base: "1",
			want: "13", // 1*7 = 7, check = (10 - 7) = 3
		},
		{
			name: "two digits",
			base: "12",
			want: "123", // 1*1 + 2*7 = 15, check = (10 - 5) = 5
		},
		{
			name: "large invoice number",
			base: "202501234",
			want: "2025012343", // check = 3
		},
		{
			name: "all zeros",
			base: "0000",
			want: "00000", // sum = 0, check = 0
		},
		{
			name: "check digit is zero",
			base: "77",
			want: "770", // 7*7 + 7*3 = 70, check = (10 - 0) % 10 = 0
		},
		{
			name:    "empty string",
			base:    "",
			wantErr: true,
		},
		{
			name:    "contains letters",
			base:    "INV123",
			wantErr: true,
		},
		{
			name:    "contains spaces",
			base:    "12 34",
			wantErr: true,
		},
		{
			name:    "contains special characters",
			base:    "123-456",
			wantErr: true,
		},
		{
			name: "whitespace trimmed",
			base: "  1234  ",
			want: "12344",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateEstonianReference(tt.base)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateEstonianReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GenerateEstonianReference(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestValidateEstonianReference(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		want      bool
	}{
		{
			name:      "valid reference",
			reference: "12344",
			want:      true,
		},
		{
			name:      "valid reference - known example",
			reference: "121312952",
			want:      true,
		},
		{
			name:      "valid reference - another example",
			reference: "1234561",
			want:      true,
		},
		{
			name:      "invalid check digit",
			reference: "12348",
			want:      false,
		},
		{
			name:      "wrong check digit",
			reference: "121312953",
			want:      false,
		},
		{
			name:      "too short",
			reference: "1",
			want:      false,
		},
		{
			name:      "empty string",
			reference: "",
			want:      false,
		},
		{
			name:      "contains letters",
			reference: "INV12347",
			want:      false,
		},
		{
			name:      "contains spaces",
			reference: "123 47",
			want:      false,
		},
		{
			name:      "valid with whitespace",
			reference: "  12344  ",
			want:      true,
		},
		{
			name:      "valid single digit base",
			reference: "13",
			want:      true,
		},
		{
			name:      "valid check digit zero",
			reference: "770",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateEstonianReference(tt.reference)
			if got != tt.want {
				t.Errorf("ValidateEstonianReference(%q) = %v, want %v", tt.reference, got, tt.want)
			}
		})
	}
}

func TestCalculateEstonian371CheckDigit(t *testing.T) {
	tests := []struct {
		name string
		base string
		want int
	}{
		{
			name: "example 1",
			base: "1234",
			want: 4,
		},
		{
			name: "example 2",
			base: "12131295",
			want: 2,
		},
		{
			name: "example 3",
			base: "123456",
			want: 1,
		},
		{
			name: "single digit",
			base: "1",
			want: 3,
		},
		{
			name: "check digit is zero",
			base: "77",
			want: 0,
		},
		{
			name: "all zeros",
			base: "0000",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateEstonian371CheckDigit(tt.base)
			if got != tt.want {
				t.Errorf("calculateEstonian371CheckDigit(%q) = %v, want %v", tt.base, got, tt.want)
			}
		})
	}
}

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	bases := []string{
		"1",
		"12",
		"123",
		"1234",
		"12345",
		"123456",
		"1234567",
		"100001",
		"202501234",
		"999999999",
	}

	for _, base := range bases {
		t.Run(base, func(t *testing.T) {
			// Generate reference
			ref, err := GenerateEstonianReference(base)
			if err != nil {
				t.Fatalf("GenerateEstonianReference(%q) error = %v", base, err)
			}

			// Validate it
			if !ValidateEstonianReference(ref) {
				t.Errorf("Generated reference %q for base %q failed validation", ref, base)
			}
		})
	}
}

func BenchmarkGenerateEstonianReference(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateEstonianReference("202501234")
	}
}

func BenchmarkValidateEstonianReference(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateEstonianReference("2025012344")
	}
}
