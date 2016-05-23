package common

import (
	"testing"
	"strings"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

func TestGetCollectorIdFromExistingFile(t *testing.T) {
	content := []byte(" 2135792e-8556-4bf0-8aef-503f29890b09 \n")
	tmpfile, err := ioutil.TempFile("", "test-collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}

	result := GetCollectorId("file:/" + tmpfile.Name())

	expect := "2135792e-8556-4bf0-8aef-503f29890b09"
	if !strings.Contains(result, expect) {
		t.Fail()
	}
}

func TestGetCollectorIdFromNonExistingFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	tmpfile := filepath.Join(dir, "collector-id")
	result := GetCollectorId("file:/" + tmpfile)
	match, err := regexp.Match("^[0-9a-f]{8}-", []byte(result))
	if err != nil {
		t.Fatal(err)
	}

	if !match {
		t.Fail()
	}
}
