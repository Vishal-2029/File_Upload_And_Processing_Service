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

// extractPages reads clean text from each PDF page.
// For simple PDFs it uses the standard font lookup; for complex PDFs (Canva, Word with CID fonts)
// it falls back to traversing Form XObjects which nest the actual content.
func extractPages(filePath string) []PageText {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	pages := make([]PageText, r.NumPage())
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		text := extractPageText(p)
		pages[i-1] = PageText{Page: i, Text: strings.TrimRight(text, "\n")}
	}
	return pages
}

// extractPageText tries the standard approach first, then falls back to XObject traversal.
func extractPageText(p pdf.Page) string {
	if p.V.IsNull() {
		return ""
	}

	// Standard path: fonts are in page-level resources.
	fonts := buildFontMap(p.Resources().Key("Font"))
	if len(fonts) > 0 {
		text, _ := p.GetPlainText(fonts)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}

	// Fallback: content is wrapped in Form XObjects (Canva, some Word PDFs).
	return extractFromXObjects(p.Resources())
}

// buildFontMap builds a map of font name → *pdf.Font from a PDF font dictionary Value.
func buildFontMap(fontDict pdf.Value) map[string]*pdf.Font {
	fonts := make(map[string]*pdf.Font)
	for _, name := range fontDict.Keys() {
		fv := fontDict.Key(name)
		f := pdf.Font{V: fv}
		fonts[name] = &f
	}
	return fonts
}

// extractFromXObjects traverses Form XObjects in a resource dictionary
// and extracts text from each one using the XObject's own font resources.
func extractFromXObjects(res pdf.Value) string {
	xobj := res.Key("XObject")
	if xobj.Kind() == pdf.Null {
		return ""
	}

	var result strings.Builder
	for _, key := range xobj.Keys() {
		x := xobj.Key(key)
		if x.Key("Subtype").Name() != "Form" {
			continue
		}

		xres := x.Key("Resources")
		fonts := buildFontMap(xres.Key("Font"))

		// Recursively handle nested XObjects too.
		nested := extractFromXObjects(xres)
		if nested != "" {
			result.WriteString(nested)
		}

		text := interpretContentStream(x, fonts)
		result.WriteString(text)
	}
	return result.String()
}

// interpretContentStream runs pdf.Interpret on a content stream Value
// and extracts text using the provided font map.
func interpretContentStream(strm pdf.Value, fonts map[string]*pdf.Font) string {
	var enc pdf.TextEncoding = &passthroughEncoding{}
	var buf strings.Builder

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "BT":
			buf.WriteByte('\n')
		case "T*":
			buf.WriteByte('\n')
		case "Tf":
			if len(args) == 2 {
				if font, ok := fonts[args[0].Name()]; ok {
					enc = font.Encoder()
				} else {
					enc = &passthroughEncoding{}
				}
			}
		case "Tj", "'":
			if len(args) >= 1 {
				buf.WriteString(enc.Decode(args[len(args)-1].RawString()))
			}
		case "\"":
			if len(args) == 3 {
				buf.WriteString(enc.Decode(args[2].RawString()))
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for j := 0; j < v.Len(); j++ {
					elem := v.Index(j)
					if elem.Kind() == pdf.String {
						buf.WriteString(enc.Decode(elem.RawString()))
					}
				}
			}
		}
	})

	return buf.String()
}

// passthroughEncoding returns raw bytes as-is (used before a font is set).
type passthroughEncoding struct{}

func (e *passthroughEncoding) Decode(raw string) string { return raw }

// extractText returns all pages joined with page-separator markers.
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
	parts := strings.Split(text, "\n\n[PAGE ")
	pages := make([]PageText, 0, len(parts))
	for i, part := range parts {
		if i == 0 {
			pages = append(pages, PageText{Page: 1, Text: strings.TrimSpace(part)})
			continue
		}
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

// GeneratePDF creates a multi-page PDF from the stored extracted_text.
func GeneratePDF(text, outputPath string) error {
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetFont("Arial", "", 11)
	doc.SetMargins(20, 20, 20)

	pages := ParsePages(text)
	if len(pages) == 0 {
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
