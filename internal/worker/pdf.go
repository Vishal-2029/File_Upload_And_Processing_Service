package worker

import (
	"os"
	"path/filepath"
	"strings"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/Vishal-2029/file-upload-service/internal/models"
)

// ProcessPDF extracts page count and approximates word count from a PDF file.
// Returns FileMeta with PageCount and WordCount populated.
func ProcessPDF(filePath string) (models.FileMeta, error) {
	conf := pdfmodel.NewDefaultConfiguration()

	pageCount, err := pdfapi.PageCountFile(filePath)
	if err != nil {
		return models.FileMeta{}, err
	}

	wordCount := extractWordCount(filePath, conf)

	return models.FileMeta{
		PageCount: pageCount,
		WordCount: wordCount,
	}, nil
}

// extractWordCount extracts content streams and counts whitespace-delimited tokens.
// Non-fatal: returns 0 on any error so a page count is still recorded.
func extractWordCount(filePath string, conf *pdfmodel.Configuration) int {
	tmpDir, err := os.MkdirTemp("", "pdfextract-*")
	if err != nil {
		return 0
	}
	defer os.RemoveAll(tmpDir)

	if err := pdfapi.ExtractContentFile(filePath, tmpDir, nil, conf); err != nil {
		return 0
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return 0
	}

	wordCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tmpDir, entry.Name()))
		if err != nil {
			continue
		}
		// Content streams contain PostScript operators mixed with text.
		// Count non-operator tokens as a rough word approximation.
		for _, tok := range strings.Fields(string(data)) {
			if len(tok) > 1 && !isOperator(tok) {
				wordCount++
			}
		}
	}
	return wordCount
}

// isOperator returns true for single-character PostScript operators.
func isOperator(s string) bool {
	operators := map[string]bool{
		"BT": true, "ET": true, "Tf": true, "Td": true, "TD": true,
		"Tm": true, "T*": true, "Tj": true, "TJ": true, "Tr": true,
		"Ts": true, "Tw": true, "Tz": true, "cm": true, "q":  true,
		"Q":  true, "re": true, "f":  true, "S":  true, "n":  true,
	}
	return operators[s]
}
