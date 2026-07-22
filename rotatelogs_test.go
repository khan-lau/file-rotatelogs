package rotatelogs_test

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	rotatelogs "github.com/khan-lau/file-rotatelogs"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestSatisfiesIOWriter(t *testing.T) {
	var w io.Writer
	w, _ = rotatelogs.New("/foo/bar")
	_ = w
}

func TestSatisfiesIOCloser(t *testing.T) {
	var c io.Closer
	c, _ = rotatelogs.New("/foo/bar")
	_ = c
}

func TestLogRotate(t *testing.T) {
	testCases := []struct {
		Name        string
		FixArgs     func([]rotatelogs.Option, string) []rotatelogs.Option
		CheckExtras func(*testing.T, *rotatelogs.RotateLogs, string) bool
	}{
		{
			Name: "Basic Usage",
		},
		{
			Name: "With Symlink",
			FixArgs: func(options []rotatelogs.Option, dir string) []rotatelogs.Option {
				linkName := filepath.Join(dir, "log")

				return append(options, rotatelogs.WithLinkName(linkName))
			},
			CheckExtras: func(t *testing.T, rl *rotatelogs.RotateLogs, dir string) bool {
				linkName := filepath.Join(dir, "log")
				linkDest, err := os.Readlink(linkName)
				if !assert.NoError(t, err, `os.Readlink(%#v) should succeed`, linkName) {
					return false
				}

				expectedLinkDest := filepath.Base(rl.CurrentFileName())
				t.Logf("expecting relative link: %s", expectedLinkDest)

				return assert.Equal(t, linkDest, expectedLinkDest, `Symlink destination should  match expected filename (%#v != %#v)`, expectedLinkDest, linkDest)
			},
		},
		{
			Name: "With Symlink (multiple levels)",
			FixArgs: func(options []rotatelogs.Option, dir string) []rotatelogs.Option {
				linkName := filepath.Join(dir, "nest1", "nest2", "log")

				return append(options, rotatelogs.WithLinkName(linkName))
			},
			CheckExtras: func(t *testing.T, rl *rotatelogs.RotateLogs, dir string) bool {
				linkName := filepath.Join(dir, "nest1", "nest2", "log")
				linkDest, err := os.Readlink(linkName)
				if !assert.NoError(t, err, `os.Readlink(%#v) should succeed`, linkName) {
					return false
				}

				expectedLinkDest := filepath.Join("..", "..", filepath.Base(rl.CurrentFileName()))
				t.Logf("expecting relative link: %s", expectedLinkDest)

				return assert.Equal(t, linkDest, expectedLinkDest, `Symlink destination should  match expected filename (%#v != %#v)`, expectedLinkDest, linkDest)
			},
		},
	}

	for i, tc := range testCases {
		i := i   // avoid lint errors
		tc := tc // avoid lint errors
		t.Run(tc.Name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", fmt.Sprintf("file-rotatelogs-test%d", i))
			if !assert.NoError(t, err, "creating temporary directory should succeed") {
				return
			}
			defer os.RemoveAll(dir)

			// Change current time, so we can safely purge old logs
			dummyTime := time.Now().Add(-7 * 24 * time.Hour)
			dummyTime = dummyTime.Add(time.Duration(-1 * dummyTime.Nanosecond()))
			clock := clockwork.NewFakeClockAt(dummyTime)

			options := []rotatelogs.Option{rotatelogs.WithClock(clock), rotatelogs.WithMaxAge(24 * time.Hour)}
			if fn := tc.FixArgs; fn != nil {
				options = fn(options, dir)
			}

			rl, err := rotatelogs.New(filepath.Join(dir, "log%Y%m%d%H%M%S"), options...)
			if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
				return
			}
			defer rl.Close()

			str := "Hello, World"
			n, err := rl.Write([]byte(str))
			if !assert.NoError(t, err, "rl.Write should succeed") {
				return
			}

			if !assert.Len(t, str, n, "rl.Write should succeed") {
				return
			}

			fn := rl.CurrentFileName()
			if fn == "" {
				t.Errorf("Could not get filename %s", fn)
			}

			content, err := os.ReadFile(fn)
			if err != nil {
				t.Errorf("Failed to read file %s: %s", fn, err)
			}

			if string(content) != str {
				t.Errorf(`File content does not match (was "%s")`, content)
			}

			err = os.Chtimes(fn, dummyTime, dummyTime)
			if err != nil {
				t.Errorf("Failed to change access/modification times for %s: %s", fn, err)
			}

			fi, err := os.Stat(fn)
			if err != nil {
				t.Errorf("Failed to stat %s: %s", fn, err)
			}

			if !fi.ModTime().Equal(dummyTime) {
				t.Errorf("Failed to chtime for %s (expected %s, got %s)", fn, fi.ModTime(), dummyTime)
			}

			clock.Advance(7 * 24 * time.Hour)

			// This next Write() should trigger Rotate()
			rl.Write([]byte(str))
			newfn := rl.CurrentFileName()
			if newfn == fn {
				t.Errorf(`New file name and old file name should not match ("%s" != "%s")`, fn, newfn)
			}

			content, err = os.ReadFile(newfn)
			if err != nil {
				t.Errorf("Failed to read file %s: %s", newfn, err)
			}

			if string(content) != str {
				t.Errorf(`File content does not match (was "%s")`, content)
			}

			time.Sleep(time.Second)

			// fn was declared above, before mocking CurrentTime
			// Old files should have been unlinked
			_, err = os.Stat(fn)
			if !assert.Error(t, err, "os.Stat should have failed") {
				return
			}

			if fn := tc.CheckExtras; fn != nil {
				if !fn(t, rl, dir) {
					return
				}
			}
		})
	}
}

func CreateRotationTestFile(dir string, base time.Time, d time.Duration, n int) {
	timestamp := base
	for i := 0; i < n; i++ {
		// %Y%m%d%H%M%S
		suffix := timestamp.Format("20060102150405")
		path := filepath.Join(dir, "log"+suffix)
		os.WriteFile(path, []byte("rotation test file\n"), os.ModePerm)
		os.Chtimes(path, timestamp, timestamp)
		timestamp = timestamp.Add(d)
	}
}

func TestLogRotationCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-rotationcount-test")
	if !assert.NoError(t, err, "creating temporary directory should succeed") {
		return
	}
	defer os.RemoveAll(dir)

	dummyTime := time.Now().Add(-7 * 24 * time.Hour)
	dummyTime = dummyTime.Add(time.Duration(-1 * dummyTime.Nanosecond()))
	clock := clockwork.NewFakeClockAt(dummyTime)

	t.Run("Either maxAge or rotationCount should be set", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "log%Y%m%d%H%M%S"),
			rotatelogs.WithClock(clock),
			rotatelogs.WithMaxAge(time.Duration(0)),
			rotatelogs.WithRotationCount(0),
		)
		if !assert.NoError(t, err, `Both of maxAge and rotationCount is disabled`) {
			return
		}
		defer rl.Close()
	})

	t.Run("Either maxAge or rotationCount should be set", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "log%Y%m%d%H%M%S"),
			rotatelogs.WithClock(clock),
			rotatelogs.WithMaxAge(1),
			rotatelogs.WithRotationCount(1),
		)
		if !assert.Error(t, err, `Both of maxAge and rotationCount is enabled`) {
			return
		}
		if rl != nil {
			defer rl.Close()
		}
	})

	t.Run("Only latest log file is kept", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "log%Y%m%d%H%M%S"),
			rotatelogs.WithClock(clock),
			rotatelogs.WithMaxAge(-1),
			rotatelogs.WithRotationCount(1),
		)
		if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
			return
		}
		defer rl.Close()

		n, err := rl.Write([]byte("dummy"))
		if !assert.NoError(t, err, "rl.Write should succeed") {
			return
		}
		if !assert.Len(t, "dummy", n, "rl.Write should succeed") {
			return
		}
		time.Sleep(time.Second)
		files, _ := filepath.Glob(filepath.Join(dir, "log*"))
		if !assert.Equal(t, 1, len(files), "Only latest log is kept") {
			return
		}
	})

	t.Run("Old log files are purged except 2 log files", func(t *testing.T) {
		CreateRotationTestFile(dir, dummyTime, time.Hour, 5)
		rl, err := rotatelogs.New(
			filepath.Join(dir, "log%Y%m%d%H%M%S"),
			rotatelogs.WithClock(clock),
			rotatelogs.WithMaxAge(-1),
			rotatelogs.WithRotationCount(2),
		)
		if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
			return
		}
		defer rl.Close()

		n, err := rl.Write([]byte("dummy"))
		if !assert.NoError(t, err, "rl.Write should succeed") {
			return
		}
		if !assert.Len(t, "dummy", n, "rl.Write should succeed") {
			return
		}
		time.Sleep(time.Second)
		files, _ := filepath.Glob(filepath.Join(dir, "log*"))
		if !assert.Equal(t, 2, len(files), "One file is kept") {
			return
		}
	})
}

func TestLogSetOutput(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-test")
	if err != nil {
		t.Errorf("Failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(dir)

	rl, err := rotatelogs.New(filepath.Join(dir, "log%Y%m%d%H%M%S"))
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}
	defer rl.Close()

	log.SetOutput(rl)
	defer log.SetOutput(os.Stderr)

	str := "Hello, World"
	log.Print(str)

	fn := rl.CurrentFileName()
	if fn == "" {
		t.Errorf("Could not get filename %s", fn)
	}

	content, err := os.ReadFile(fn)
	if err != nil {
		t.Errorf("Failed to read file %s: %s", fn, err)
	}

	if !strings.Contains(string(content), str) {
		t.Errorf(`File content does not contain "%s" (was "%s")`, str, content)
	}
}

func TestHandlerCallback(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-handler-test")
	if !assert.NoError(t, err, "creating temporary directory should succeed") {
		return
	}
	defer os.RemoveAll(dir)

	type capturedEvent struct {
		prev    string
		current string
		eType   rotatelogs.EventType
	}

	done := make(chan capturedEvent, 1)
	rl, err := rotatelogs.New(
		filepath.Join(dir, "log%Y%m%d%H%M%S"),
		rotatelogs.WithHandler(rotatelogs.HandlerFunc(func(e rotatelogs.Event) {
			ev := e.(*rotatelogs.FileRotatedEvent)
			done <- capturedEvent{
				prev:    ev.PreviousFile(),
				current: ev.CurrentFile(),
				eType:   ev.Type(),
			}
		})),
		rotatelogs.WithRotationTime(time.Nanosecond),
	)
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}
	defer rl.Close()

	// First write creates the initial log file, which also triggers the handler
	rl.Write([]byte("Hello, World"))
	fn1 := rl.CurrentFileName()
	t.Logf("First file: %s", fn1)

	// Drain the handler event from initial file creation
	<-done

	// Force rotation
	if !assert.NoError(t, rl.Rotate(), "rl.Rotate should succeed") {
		return
	}

	// Wait for handler callback from rotation
	var ev capturedEvent
	select {
	case ev = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for handler callback")
	}

	fn2 := rl.CurrentFileName()
	t.Logf("After rotation: %s", fn2)

	assert.Equal(t, rotatelogs.FileRotatedEventType, ev.eType, "event type should be FileRotatedEventType")
	assert.Equal(t, fn1, ev.prev, "previous file should match")
	assert.Equal(t, fn2, ev.current, "current file should match")
}

func TestWithNamingFunc(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-naming-test")
	if !assert.NoError(t, err, "creating temporary directory should succeed") {
		return
	}
	defer os.RemoveAll(dir)

	var (
		capturedBase string
		capturedGen  int
	)

	rl, err := rotatelogs.New(
		filepath.Join(dir, "test.log"),
		rotatelogs.ForceNewFile(),
		rotatelogs.WithNamingFunc(func(baseFilename string, generation int) string {
			capturedBase = baseFilename
			capturedGen = generation
			// Custom naming: app.log.N (suffix after extension, like old style)
			return fmt.Sprintf("%s.%d", baseFilename, generation)
		}),
	)
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}
	defer rl.Close()

	// First write creates test.log (generation 0, no suffix)
	rl.Write([]byte("Hello, World"))

	// Close and re-open with ForceNewFile to trigger rotation
	rl.Close()
	rl, err = rotatelogs.New(
		filepath.Join(dir, "test.log"),
		rotatelogs.ForceNewFile(),
		rotatelogs.WithNamingFunc(func(baseFilename string, generation int) string {
			capturedBase = baseFilename
			capturedGen = generation
			return fmt.Sprintf("%s.%d", baseFilename, generation)
		}),
	)
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}
	defer rl.Close()

	rl.Write([]byte("Hello, World"))

	expectedFn := filepath.Join(dir, "test.log.1")
	assert.FileExists(t, expectedFn, "custom named file should exist")
	assert.Equal(t, filepath.Join(dir, "test.log"), capturedBase, "should receive base filename")
	assert.Equal(t, 1, capturedGen, "should receive generation 1")
	assert.Equal(t, expectedFn, rl.CurrentFileName(), "current file should use custom name")
}

func TestWithAgingFunc(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-aging-test")
	if !assert.NoError(t, err, "creating temporary directory should succeed") {
		return
	}
	defer os.RemoveAll(dir)

	// Pre-create files that match the glob pattern
	if !assert.NoError(t, os.WriteFile(filepath.Join(dir, "test.log"), []byte("current"), os.ModePerm)) {
		return
	}
	if !assert.NoError(t, os.WriteFile(filepath.Join(dir, "test.log.1"), []byte("old1"), os.ModePerm)) {
		return
	}
	if !assert.NoError(t, os.WriteFile(filepath.Join(dir, "test.log.2"), []byte("old2"), os.ModePerm)) {
		return
	}

	var capturedFiles []rotatelogs.LogFileInfo
	var mu sync.Mutex

	rl, err := rotatelogs.New(
		filepath.Join(dir, "test.log"),
		rotatelogs.ForceNewFile(),
		rotatelogs.WithAgingFunc(func(files []rotatelogs.LogFileInfo) []string {
			mu.Lock()
			capturedFiles = append(capturedFiles, files...)
			mu.Unlock()

			// Delete test.log.1 and test.log.2
			var toDelete []string
			for _, f := range files {
				if strings.HasSuffix(f.Path, ".1") || strings.HasSuffix(f.Path, ".2") {
					toDelete = append(toDelete, f.Path)
				}
			}
			return toDelete
		}),
	)
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}
	defer rl.Close()

	// Trigger rotation
	_, err = rl.Write([]byte("trigger"))
	if !assert.NoError(t, err, "rl.Write should succeed") {
		return
	}

	// AgingFunc should have been called with matched files
	mu.Lock()
	capturedCount := len(capturedFiles)
	mu.Unlock()
	if !assert.True(t, capturedCount > 0, "aging func should have been called with files") {
		return
	}

	// test.log.1 and test.log.2 should be deleted (async, so retry)
	assertDeleted := func(path string) bool {
		for i := 0; i < 30; i++ {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	assert.True(t, assertDeleted(filepath.Join(dir, "test.log.1")), "test.log.1 should be deleted by aging func")
	assert.True(t, assertDeleted(filepath.Join(dir, "test.log.2")), "test.log.2 should be deleted by aging func")
}

func TestGHIssue16(t *testing.T) {
	defer func() {
		if v := recover(); v != nil {
			assert.NoError(t, errors.Errorf("%s", v), "error should be nil")
		}
	}()

	dir, err := os.MkdirTemp("", "file-rotatelogs-gh16")
	if !assert.NoError(t, err, `creating temporary directory should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	rl, err := rotatelogs.New(
		filepath.Join(dir, "log%Y%m%d%H%M%S"),
		rotatelogs.WithLinkName("./test.log"),
		rotatelogs.WithRotationTime(10*time.Second),
		rotatelogs.WithRotationCount(3),
		rotatelogs.WithMaxAge(-1),
	)
	if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
		return
	}

	if !assert.NoError(t, rl.Rotate(), "rl.Rotate should succeed") {
		return
	}
	defer rl.Close()
}

func TestRotationGenerationalNames(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-generational")
	if !assert.NoError(t, err, `creating temporary directory should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	t.Run("Rotate over unchanged pattern", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "unchanged-pattern.log"),
		)
		if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
			return
		}

		seen := map[string]struct{}{}
		for i := 0; i < 10; i++ {
			rl.Write([]byte("Hello, World!"))
			if !assert.NoError(t, rl.Rotate(), "rl.Rotate should succeed") {
				return
			}

			// Because every call to Rotate should yield a new log file,
			// and the previous files already exist, the filenames should share
			// the same prefix and have a unique suffix
			fn := filepath.Base(rl.CurrentFileName())
			if !assert.True(t, strings.HasPrefix(fn, "unchanged-pattern"), "prefix for all filenames should match") {
				return
			}
			rl.Write([]byte("Hello, World!"))
			// new naming: "unchanged-pattern.1.log" → extract ".1"
			suffix := strings.TrimSuffix(strings.TrimPrefix(fn, "unchanged-pattern"), ".log")
			expectedSuffix := fmt.Sprintf(".%d", i+1)
			if !assert.True(t, suffix == expectedSuffix, "expected suffix %s found %s", expectedSuffix, suffix) {
				return
			}
			assert.FileExists(t, rl.CurrentFileName(), "file does not exist %s", rl.CurrentFileName())
			stat, err := os.Stat(rl.CurrentFileName())
			if err == nil {
				if !assert.True(t, stat.Size() == 13, "file %s size is %d, expected 13", rl.CurrentFileName(), stat.Size()) {
					return
				}
			} else {
				assert.Failf(t, "could not stat file %s", rl.CurrentFileName())

				return
			}

			if _, ok := seen[suffix]; !assert.False(t, ok, `filename suffix %s should be unique`, suffix) {
				return
			}
			seen[suffix] = struct{}{}
		}
		defer rl.Close()
	})
	t.Run("Rotate over pattern change over every second", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "every-second-pattern-%Y%m%d%H%M%S.log"),
			rotatelogs.WithRotationTime(time.Nanosecond),
		)
		if !assert.NoError(t, err, `rotatelogs.New should succeed`) {
			return
		}

		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			rl.Write([]byte("Hello, World!"))
			if !assert.NoError(t, rl.Rotate(), "rl.Rotate should succeed") {
				return
			}

			// The pattern changes every second, so each Rotate creates a new file
			// with a different timestamp (no generation suffix needed)
			if !assert.FileExists(t, rl.CurrentFileName(), "rotated file should exist") {
				return
			}
		}
		defer rl.Close()
	})
}

type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time {
	return f()
}

func TestGHIssue23(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-generational")
	if !assert.NoError(t, err, `creating temporary directory should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	for _, locName := range []string{"Asia/Tokyo", "Pacific/Honolulu"} {
		locName := locName
		loc, _ := time.LoadLocation(locName)
		tests := []struct {
			Expected string
			Clock    rotatelogs.Clock
		}{
			{
				Expected: filepath.Join(dir, strings.ToLower(strings.Replace(locName, "/", "_", -1))+".201806010000.log"),
				Clock: ClockFunc(func() time.Time {
					return time.Date(2018, 6, 1, 3, 18, 0, 0, loc)
				}),
			},
			{
				Expected: filepath.Join(dir, strings.ToLower(strings.Replace(locName, "/", "_", -1))+".201712310000.log"),
				Clock: ClockFunc(func() time.Time {
					return time.Date(2017, 12, 31, 23, 52, 0, 0, loc)
				}),
			},
		}
		for _, test := range tests {
			test := test
			t.Run(fmt.Sprintf("location = %s, time = %s", locName, test.Clock.Now().Format(time.RFC3339)), func(t *testing.T) {
				template := strings.ToLower(strings.Replace(locName, "/", "_", -1)) + ".%Y%m%d%H%M.log"
				rl, err := rotatelogs.New(
					filepath.Join(dir, template),
					rotatelogs.WithClock(test.Clock), // we're not using WithLocation, but it's the same thing
				)
				if !assert.NoError(t, err, "rotatelogs.New should succeed") {
					return
				}

				t.Logf("expected %s", test.Expected)
				rl.Rotate()
				if !assert.Equal(t, test.Expected, rl.CurrentFileName(), "file names should match") {
					return
				}
			})
		}
	}
}

func TestForceNewFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-rotatelogs-force-new-file")
	if !assert.NoError(t, err, `creating temporary directory should succeed`) {
		return
	}
	defer os.RemoveAll(dir)

	t.Run("Force a new file", func(t *testing.T) {
		rl, err := rotatelogs.New(
			filepath.Join(dir, "force-new-file.log"),
			rotatelogs.ForceNewFile(),
		)
		if !assert.NoError(t, err, "rotatelogs.New should succeed") {
			return
		}
		rl.Write([]byte("Hello, World!"))
		rl.Close()

		for i := 0; i < 10; i++ {
			baseFn := filepath.Join(dir, "force-new-file.log")
			rl, err := rotatelogs.New(
				baseFn,
				rotatelogs.ForceNewFile(),
			)
			if !assert.NoError(t, err, "rotatelogs.New should succeed") {
				return
			}
			rl.Write([]byte("Hello, World"))
			rl.Write([]byte(fmt.Sprintf("%d", i)))
			rl.Close()

			fn := filepath.Base(rl.CurrentFileName())
			// new naming: "force-new-file.1.log" → extract ".1"
			suffix := strings.TrimSuffix(strings.TrimPrefix(fn, "force-new-file"), ".log")
			expectedSuffix := fmt.Sprintf(".%d", i+1)
			if !assert.True(t, suffix == expectedSuffix, "expected suffix %s found %s", expectedSuffix, suffix) {
				return
			}
			assert.FileExists(t, rl.CurrentFileName(), "file does not exist %s", rl.CurrentFileName())
			content, err := os.ReadFile(rl.CurrentFileName())
			if !assert.NoError(t, err, "os.ReadFile %s should succeed", rl.CurrentFileName()) {
				return
			}
			str := fmt.Sprintf("Hello, World%d", i)
			if !assert.Equal(t, str, string(content), "read %s from file %s, not expected %s", string(content), rl.CurrentFileName(), str) {
				return
			}

			assert.FileExists(t, baseFn, "file does not exist %s", baseFn)
			content, err = os.ReadFile(baseFn)
			if !assert.NoError(t, err, "os.ReadFile should succeed") {
				return
			}
			if !assert.Equal(t, "Hello, World!", string(content), "read %s from file %s, not expected Hello, World!", string(content), baseFn) {
				return
			}
		}
	})

	t.Run("Force a new file with Rotate", func(t *testing.T) {
		baseFn := filepath.Join(dir, "force-new-file-rotate.log")
		rl, err := rotatelogs.New(
			baseFn,
			rotatelogs.ForceNewFile(),
		)
		if !assert.NoError(t, err, "rotatelogs.New should succeed") {
			return
		}
		rl.Write([]byte("Hello, World!"))

		for i := 0; i < 10; i++ {
			if !assert.NoError(t, rl.Rotate(), "rl.Rotate should succeed") {
				return
			}
			rl.Write([]byte("Hello, World"))
			fmt.Fprintf(rl, "%d", i)
			assert.FileExists(t, rl.CurrentFileName(), "file does not exist %s", rl.CurrentFileName())
			content, err := os.ReadFile(rl.CurrentFileName())
			if !assert.NoError(t, err, "os.ReadFile %s should succeed", rl.CurrentFileName()) {
				return
			}
			str := fmt.Sprintf("Hello, World%d", i)
			if !assert.Equal(t, str, string(content), "read %s from file %s, not expected %s", string(content), rl.CurrentFileName(), str) {
				return
			}

			assert.FileExists(t, baseFn, "file does not exist %s", baseFn)
			content, err = os.ReadFile(baseFn)
			if !assert.NoError(t, err, "os.ReadFile should succeed") {
				return
			}
			if !assert.Equal(t, "Hello, World!", string(content), "read %s from file %s, not expected Hello, World!", string(content), baseFn) {
				return
			}
		}
	})
}
