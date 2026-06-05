package service

import "testing"

func TestResumeNeedsFullScanWhenHistoricalScanWasIncomplete(t *testing.T) {
	task := map[string]interface{}{
		"taskNum": `{"scanFinish":false,"scan":{"scannedDirs":3,"remainingDirs":2,"totalDirs":5}}`,
	}

	if !resumeNeedsFullScan(task) {
		t.Fatalf("resumeNeedsFullScan() = false, want true for incomplete scan")
	}
}

func TestResumeUsesRecordedItemsWhenHistoricalScanCompleted(t *testing.T) {
	task := map[string]interface{}{
		"taskNum": `{"scanFinish":true,"scan":{"scannedDirs":5,"remainingDirs":0,"totalDirs":5}}`,
	}

	if resumeNeedsFullScan(task) {
		t.Fatalf("resumeNeedsFullScan() = true, want false for completed scan")
	}
}

func TestResumeNeedsFullScanWhenHistoricalScanStateIsUnknown(t *testing.T) {
	for _, taskNum := range []interface{}{nil, "", `{"successNum":1}`, `not-json`} {
		task := map[string]interface{}{"taskNum": taskNum}

		if !resumeNeedsFullScan(task) {
			t.Fatalf("resumeNeedsFullScan(%v) = false, want true when scan state is unknown", taskNum)
		}
	}
}
