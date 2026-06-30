package worker

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/ledongthuc/pdf"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"

	"github.com/Vishal-2029/file-upload-service/internal/models"
)

// PDFResult holds both the metadata and extracted text from a PDF.
type PDFResult struct {
	Meta          models.FileMeta
	ExtractedText string
}

// ProcessPDF extracts page count, word count, and full text from a PDF file.
func ProcessPDF(filePath string) (PDFResult, error) {
	pageCount, err := pdfapi.PageCountFile(filePath)
	if err != nil {
		return PDFResult{}, err
	}

	text := extractText(filePath)
	wordCount := len(strings.Fields(text))

	return PDFResult{
		Meta: models.FileMeta{
			PageCount: pageCount,
			WordCount: wordCount,
		},
		ExtractedText: text,
	}, nil
}

// PageText holds the text content of a single PDF page.
type PageText struct {
	Page int    `json:"page"`
	Text string `json:"text"`
}

// extractPages reads clean text from each page separately.
// Returns one PageText per page; empty pages are included so page numbers stay stable.
func extractPages(filePath string) []PageText {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	pages := make([]PageText, r.NumPage())
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		text := ""
		if !p.V.IsNull() {
			text, _ = p.GetPlainText(nil)
		}
		pages[i-1] = PageText{Page: i, Text: strings.TrimRight(text, "\n")}
	}
	return pages
}

// extractText returns all pages joined with page-separator markers.
// Used for backwards-compatible single-blob storage in extracted_text column.
func extractText(filePath string) string {
	pages := extractPages(filePath)
	var buf bytes.Buffer
	for i, p := range pages {
		if i > 0 {
			buf.WriteString(fmt.Sprintf("\n\n[PAGE %d]\n\n", p.Page))
		}
		buf.WriteString(p.Text)
	}
	return buf.String()
}

// ParsePages splits stored extracted_text back into per-page slices.
func ParsePages(text string) []PageText {
	if text == "" {
		return nil
	}
	// Split on the [PAGE N] markers inserted by extractText.
	parts := strings.Split(text, "\n\n[PAGE ")
	pages := make([]PageText, 0, len(parts))
	for i, part := range parts {
		if i == 0 {
			pages = append(pages, PageText{Page: 1, Text: strings.TrimSpace(part)})
			continue
		}
		// part starts with "N]\n\ncontent"
		idx := strings.Index(part, "]")
		if idx < 0 {
			continue
		}
		pageNum := 0
		fmt.Sscanf(part[:idx], "%d", &pageNum)
		content := strings.TrimSpace(part[idx+1:])
		pages = append(pages, PageText{Page: pageNum, Text: content})
	}
	return pages
}

// GeneratePDF creates a proper multi-page PDF from the stored extracted_text.
// Each [PAGE N] marker starts a new PDF page.
func GeneratePDF(text, outputPath string) error {
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetFont("Arial", "", 11)
	doc.SetMargins(20, 20, 20)

	pages := ParsePages(text)
	if len(pages) == 0 {
		// Fallback: treat entire text as one page.
		pages = []PageText{{Page: 1, Text: text}}
	}

	for _, p := range pages {
		doc.AddPage()
		for _, line := range strings.Split(p.Text, "\n") {
			doc.MultiCell(0, 6, line, "", "L", false)
		}
	}

	return doc.OutputFileAndClose(outputPath)
}

