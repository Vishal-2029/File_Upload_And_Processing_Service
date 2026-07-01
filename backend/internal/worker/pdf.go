package worker

import (
	"bytes"
	"fmt"
	"math"
	"sort"
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

// ── Matrix math ────────────────────────────────────────────────────────────────
// PDF uses a 3×3 matrix with row-vector convention: p' = p × M
// The 6-element form [a b c d e f] maps to:
//   M = [[a, b, 0],
//        [c, d, 0],
//        [e, f, 1]]
// Point transform: (x', y') = (x*a + y*c + e, x*b + y*d + f)

type mat3 [3][3]float64

var identMat = mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func newMat(a, b, c, d, e, f float64) mat3 {
	return mat3{{a, b, 0}, {c, d, 0}, {e, f, 1}}
}

func (x mat3) mul(y mat3) mat3 {
	var z mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				z[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	return z
}

// pointAt returns (x, y) page coordinates from row 2 of a transform result matrix.
// In the row-vector convention, Trm = scale.mul(Tm).mul(CTM); Trm[2] is the translated point.
func pointAt(m mat3) (float64, float64) { return m[2][0], m[2][1] }

// ── Text chunk ─────────────────────────────────────────────────────────────────

type textChunk struct {
	x, y float64
	text string
}

// ── Core extraction ────────────────────────────────────────────────────────────

// extractPages opens the PDF and extracts text per page in reading order.
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

func extractPageText(p pdf.Page) string {
	if p.V.IsNull() {
		return ""
	}

	// Standard path: page-level fonts (simple PDFs).
	pageFonts := buildFontMap(p.Resources().Key("Font"))
	if len(pageFonts) > 0 {
		if text, _ := p.GetPlainText(pageFonts); strings.TrimSpace(text) != "" {
			return text
		}
	}

	// Fallback: traverse Form XObjects (Canva, many Word PDFs).
	chunks := collectXObjectChunks(p.Resources(), identMat)
	return chunksToText(chunks)
}

// collectXObjectChunks recursively collects positioned text from Form XObjects.
func collectXObjectChunks(res pdf.Value, parentCTM mat3) []textChunk {
	xobj := res.Key("XObject")
	if xobj.Kind() == pdf.Null {
		return nil
	}

	var all []textChunk
	for _, key := range xobj.Keys() {
		x := xobj.Key(key)
		if x.Key("Subtype").Name() != "Form" {
			continue
		}

		xres := x.Key("Resources")
		fonts := buildFontMap(xres.Key("Font"))

		// Nested XObjects within this form.
		nested := collectXObjectChunks(xres, parentCTM)
		all = append(all, nested...)

		// Text from this form's content stream.
		chunks := interpretForChunks(x, fonts, parentCTM)
		all = append(all, chunks...)
	}
	return all
}

// interpretForChunks runs pdf.Interpret on a content stream and returns
// text chunks with their exact page-space (X, Y) positions.
func interpretForChunks(strm pdf.Value, fonts map[string]*pdf.Font, parentCTM mat3) []textChunk {
	var chunks []textChunk
	var enc pdf.TextEncoding = &passthroughEncoding{}

	ctm := parentCTM
	var ctmStack []mat3

	tm := identMat
	tlm := identMat

	addText := func(raw string) {
		decoded := enc.Decode(raw)
		if decoded == "" {
			return
		}
		// Text position: scale identity × Tm × CTM.
		trm := identMat.mul(tm).mul(ctm)
		px, py := pointAt(trm)
		chunks = append(chunks, textChunk{x: px, y: py, text: decoded})
	}

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "q":
			ctmStack = append(ctmStack, ctm)
		case "Q":
			if len(ctmStack) > 0 {
				ctm = ctmStack[len(ctmStack)-1]
				ctmStack = ctmStack[:len(ctmStack)-1]
			}
		case "cm":
			if len(args) == 6 {
				m := newMat(args[0].Float64(), args[1].Float64(),
					args[2].Float64(), args[3].Float64(),
					args[4].Float64(), args[5].Float64())
				ctm = m.mul(ctm)
			}

		case "BT":
			tm = identMat
			tlm = identMat

		case "Tf":
			if len(args) == 2 {
				if font, ok := fonts[args[0].Name()]; ok {
					enc = font.Encoder()
				} else {
					enc = &passthroughEncoding{}
				}
			}

		case "Tm":
			if len(args) == 6 {
				tm = newMat(args[0].Float64(), args[1].Float64(),
					args[2].Float64(), args[3].Float64(),
					args[4].Float64(), args[5].Float64())
				tlm = tm
			}

		case "TD", "Td":
			if len(args) == 2 {
				tx, ty := args[0].Float64(), args[1].Float64()
				shift := mat3{{1, 0, 0}, {0, 1, 0}, {tx, ty, 1}}
				tlm = shift.mul(tlm)
				tm = tlm
			}

		case "T*":
			// Move to next line using current line matrix.
			shift := mat3{{1, 0, 0}, {0, 1, 0}, {0, -12, 1}} // approx 12pt leading
			tlm = shift.mul(tlm)
			tm = tlm

		case "Tj":
			if len(args) == 1 {
				addText(args[0].RawString())
			}
		case "'":
			if len(args) == 1 {
				shift := mat3{{1, 0, 0}, {0, 1, 0}, {0, -12, 1}}
				tlm = shift.mul(tlm)
				tm = tlm
				addText(args[0].RawString())
			}
		case "\"":
			if len(args) == 3 {
				shift := mat3{{1, 0, 0}, {0, 1, 0}, {0, -12, 1}}
				tlm = shift.mul(tlm)
				tm = tlm
				addText(args[2].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for j := 0; j < v.Len(); j++ {
					elem := v.Index(j)
					if elem.Kind() == pdf.String {
						addText(elem.RawString())
					}
				}
			}
		}
	})

	return chunks
}

// chunksToText sorts text chunks into reading order and reconstructs plain text.
//
// Canva (and many modern design tools) export PDFs in Y-down coordinate space
// (origin at top-left, Y increases downward), so we sort by Y ASCENDING
// to produce top-to-bottom reading order.
func chunksToText(chunks []textChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	// Detect Y direction: find Y-range and compare to known page layout.
	// Heuristic: if yMin is close to 0 and yMax >> yMin, this is likely Y-down.
	// Either way, sort by ascending Y first and check if output makes sense.
	// For Y-up PDFs the simple library path handles it; XObject path is mainly Canva.
	yDir := yDirection(chunks)

	sort.Slice(chunks, func(i, j int) bool {
		dy := (chunks[i].y - chunks[j].y) * float64(yDir)
		if math.Abs(dy) > 4 {
			return dy < 0 // yDir=1→ascending, yDir=-1→descending
		}
		return chunks[i].x < chunks[j].x
	})

	// Group into visual lines: chunks whose Y values are within a threshold.
	lineThresh := lineGroupThreshold(chunks)
	type line struct {
		y      float64
		chunks []textChunk
	}
	var lines []line
	for _, c := range chunks {
		if len(lines) == 0 || math.Abs(c.y-lines[len(lines)-1].y) > lineThresh {
			lines = append(lines, line{y: c.y, chunks: []textChunk{c}})
		} else {
			lines[len(lines)-1].chunks = append(lines[len(lines)-1].chunks, c)
		}
	}

	var result strings.Builder
	for i, l := range lines {
		if i > 0 {
			// Extra blank line for gaps indicating section breaks.
			gap := math.Abs(lines[i].y-lines[i-1].y) * float64(yDir)
			if gap < 0 {
				gap = -gap
			}
			if gap > lineThresh*3 {
				result.WriteByte('\n')
			}
			result.WriteByte('\n')
		}

		// Sort chunks within line left-to-right.
		sort.Slice(l.chunks, func(a, b int) bool {
			return l.chunks[a].x < l.chunks[b].x
		})

		// Concatenate directly. Space characters are already included as
		// decoded chunks (Canva encodes word-spaces as explicit Tj calls),
		// so no gap-based space injection is needed.
		for _, c := range l.chunks {
			result.WriteString(c.text)
		}
	}

	return result.String()
}

// yDirection returns 1 if Y-down (sort ascending) or -1 if Y-up (sort descending).
// Canva PDFs use Y-down (origin top-left). Standard PDFs use Y-up (origin bottom-left).
func yDirection(chunks []textChunk) int {
	if len(chunks) == 0 {
		return 1
	}
	var yMin, yMax float64 = chunks[0].y, chunks[0].y
	for _, c := range chunks {
		if c.y < yMin {
			yMin = c.y
		}
		if c.y > yMax {
			yMax = c.y
		}
	}
	span := yMax - yMin
	if span == 0 {
		return 1
	}
	// If the minimum Y is close to 0 (< 5% of span), it's likely Y-down with origin at top.
	// For Y-up PDFs, text near the top of the page would have large Y values.
	if yMin >= 0 && yMin/span < 0.10 {
		return 1 // Y-down: ascending sort
	}
	return -1 // Y-up: descending sort
}

// lineGroupThreshold computes an appropriate threshold for grouping text into lines.
func lineGroupThreshold(chunks []textChunk) float64 {
	if len(chunks) < 2 {
		return 10
	}
	// Use the typical distance between adjacent Y-unique positions.
	ys := make([]float64, 0, len(chunks))
	seen := make(map[int64]bool)
	for _, c := range chunks {
		k := int64(c.y * 10)
		if !seen[k] {
			seen[k] = true
			ys = append(ys, c.y)
		}
	}
	sort.Float64s(ys)
	if len(ys) < 2 {
		return 10
	}
	// Median gap between adjacent lines.
	gaps := make([]float64, 0, len(ys)-1)
	for i := 1; i < len(ys); i++ {
		g := math.Abs(ys[i] - ys[i-1])
		if g > 0.5 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return 10
	}
	sort.Float64s(gaps)
	median := gaps[len(gaps)/2]
	// Threshold = half the typical line spacing, min 4pt.
	thresh := median * 0.5
	if thresh < 4 {
		thresh = 4
	}
	return thresh
}


// ── Helpers ────────────────────────────────────────────────────────────────────

func buildFontMap(fontDict pdf.Value) map[string]*pdf.Font {
	fonts := make(map[string]*pdf.Font)
	for _, name := range fontDict.Keys() {
		fv := fontDict.Key(name)
		f := pdf.Font{V: fv}
		fonts[name] = &f
	}
	return fonts
}

// passthroughEncoding returns raw bytes unchanged (used before a font is selected).
type passthroughEncoding struct{}

func (e *passthroughEncoding) Decode(raw string) string { return raw }

// ── Text joining ───────────────────────────────────────────────────────────────

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
