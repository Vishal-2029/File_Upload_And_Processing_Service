package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"

	"github.com/Vishal-2029/file-upload-service/internal/models"
)

// ProcessImage resizes the image to fit within 800×800 pixels (maintaining aspect ratio)
// and saves the result as JPEG. Returns the output path and metadata.
func ProcessImage(inputPath, processedDir string) (outputPath string, meta models.FileMeta, err error) {
	img, err := imaging.Open(inputPath, imaging.AutoOrientation(true))
	if err != nil {
		return "", models.FileMeta{}, fmt.Errorf("open image: %w", err)
	}

	origBounds := img.Bounds()
	origW := origBounds.Dx()
	origH := origBounds.Dy()

	// Fit inside 800×800, preserving aspect ratio.
	resized := imaging.Fit(img, 800, 800, imaging.Lanczos)

	// Build output path: replace extension with _processed.jpg
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath = filepath.Join(processedDir, base+"_processed.jpg")

	if err := imaging.Save(resized, outputPath, imaging.JPEGQuality(85)); err != nil {
		return "", models.FileMeta{}, fmt.Errorf("save image: %w", err)
	}

	return outputPath, models.FileMeta{
		Width:  origW,
		Height: origH,
		Format: "jpeg",
	}, nil
}
