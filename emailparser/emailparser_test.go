package emailparser

import (
	"fmt"
	"os"
	"testing"
)

func getEmailFilenames() []string {
	var args []string
	found := false
	for _, arg := range os.Args {
		if arg == "--" {
			found = true
			continue
		}
		if found {
			args = append(args, arg)
		}
	}
	return args
}

func testParserEmailFile(t *testing.T, emailFilename string) {
	emailData, err := os.ReadFile(emailFilename)
	if err != nil {
		t.Fatalf("read email file failed, filename: %s, err: %v", emailFilename, err)
	}
	parser := EmailParserNew(EmailParserOptions{
		DefaultCharset: "UTF-8",
		EmailData:      emailData,
	})
	fmt.Println("----- Parsing email file:", emailFilename, " -----")
	parser.DebugShow()
	fmt.Println("")
}

func TestEmailParser(t *testing.T) {
	emailFilenames := getEmailFilenames()
	for _, emailFilename := range emailFilenames {
		testParserEmailFile(t, emailFilename)
	}
}
