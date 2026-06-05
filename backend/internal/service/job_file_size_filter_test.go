package service

import "testing"

func TestJobAllowsFileSizeUsesDefaultUnlimitedRange(t *testing.T) {
	job := map[string]interface{}{}

	for _, size := range []int64{0, 1, 1024, 10 * 1024 * 1024 * 1024} {
		if !jobAllowsFileSize(job, size) {
			t.Fatalf("jobAllowsFileSize(defaults, %d) = false, want true", size)
		}
	}
}

func TestJobAllowsFileSizeRejectsFilesOutsideConfiguredRange(t *testing.T) {
	job := map[string]interface{}{
		"minFileSize": int64(1024),
		"maxFileSize": int64(4096),
	}

	cases := []struct {
		name string
		size int64
		want bool
	}{
		{name: "below minimum", size: 1023, want: false},
		{name: "at minimum", size: 1024, want: true},
		{name: "in range", size: 2048, want: true},
		{name: "at maximum", size: 4096, want: true},
		{name: "above maximum", size: 4097, want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobAllowsFileSize(job, tt.size); got != tt.want {
				t.Fatalf("jobAllowsFileSize(size=%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestJobAllowsFileSizeTreatsZeroMaxAsUnlimited(t *testing.T) {
	job := map[string]interface{}{
		"minFileSize": int64(1024),
		"maxFileSize": int64(0),
	}

	if jobAllowsFileSize(job, 1023) {
		t.Fatalf("jobAllowsFileSize(1023) = true, want false below minimum")
	}
	if !jobAllowsFileSize(job, 10*1024*1024*1024) {
		t.Fatalf("jobAllowsFileSize(10GiB) = false, want true when max is unlimited")
	}
}

func TestCleanJobInputDefaultsFileSizeRangeToUnlimited(t *testing.T) {
	job := map[string]interface{}{}

	CleanJobInput(job)

	if job["minFileSize"] != int64(0) {
		t.Fatalf("minFileSize = %#v, want int64(0)", job["minFileSize"])
	}
	if job["maxFileSize"] != int64(0) {
		t.Fatalf("maxFileSize = %#v, want int64(0)", job["maxFileSize"])
	}
}

func TestCleanJobInputRejectsInvalidFileSizeRange(t *testing.T) {
	job := map[string]interface{}{
		"minFileSize": int64(4096),
		"maxFileSize": int64(1024),
	}

	defer func() {
		if recover() == nil {
			t.Fatalf("CleanJobInput() did not panic, want validation failure")
		}
	}()

	CleanJobInput(job)
}
