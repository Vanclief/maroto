package maroto_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/johnfercher/go-tree/node"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/stretchr/testify/assert"

	"github.com/vanclief/maroto/v2"
	"github.com/vanclief/maroto/v2/pkg/components/col"
	"github.com/vanclief/maroto/v2/pkg/components/page"
	"github.com/vanclief/maroto/v2/pkg/components/row"
	"github.com/vanclief/maroto/v2/pkg/components/text"
	"github.com/vanclief/maroto/v2/pkg/config"
	"github.com/vanclief/maroto/v2/pkg/consts/pagesize"
	"github.com/vanclief/maroto/v2/pkg/core"
	"github.com/vanclief/maroto/v2/pkg/core/entity"
	"github.com/vanclief/maroto/v2/pkg/props"
)

// A 20 mm page with no margins, so the arithmetic in the boundary tests is plain.
func smallPageConfig() *entity.Config {
	return config.NewBuilder().
		WithDimensions(20, 20).
		WithTopMargin(0).
		WithBottomMargin(0).
		WithLeftMargin(0).
		WithRightMargin(0).
		Build()
}

func TestMaroto_MeasureRows(t *testing.T) {
	t.Run("when fixed rows are sent, should return the sum of their heights", func(t *testing.T) {
		sut := maroto.New()

		height := sut.MeasureRows(row.New(10).Add(col.New(12)), row.New(2.5).Add(col.New(12)))

		assert.Equal(t, 12.5, height)
	})
	t.Run("when an auto row is sent, should return the height it takes once added", func(t *testing.T) {
		sut := maroto.New()
		measured := row.New().Add(text.NewCol(4, strings.Repeat("palabra ", 20)))
		added := row.New().Add(text.NewCol(4, strings.Repeat("palabra ", 20)))

		height := sut.MeasureRows(measured)
		sut.AddRows(added)

		assert.Greater(t, height, 0.0)
		assert.Equal(t, added.GetStructure().GetData().Value, height)
	})
	t.Run("when rows are measured, should not add them to the document", func(t *testing.T) {
		measured := maroto.New(smallPageConfig())
		untouched := maroto.New(smallPageConfig())

		measured.MeasureRows(row.New(15).Add(col.New(12)), row.New(15).Add(col.New(12)))

		assert.Equal(t, rowCounts(untouched.GetStructure()), rowCounts(measured.GetStructure()))
	})
}

func TestMaroto_Fits(t *testing.T) {
	t.Run("when the rows end short of the page by more than the slack, should return true", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		sut.AddRow(19)

		assert.True(t, sut.Fits(row.New(0.9998)))
	})
	t.Run("when the rows would fill the page exactly, should return false", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		sut.AddRow(19)

		assert.False(t, sut.Fits(row.New(1)))
	})
	t.Run("when the rows fit one by one but not together, should return false", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())

		assert.True(t, sut.Fits(row.New(10)))
		assert.False(t, sut.Fits(row.New(10), row.New(10)))
	})
	t.Run("when a footer is registered, should count it", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		_ = sut.RegisterFooter(row.New(5))
		sut.AddRow(10)

		assert.True(t, sut.Fits(row.New(4.9998)))
		assert.False(t, sut.Fits(row.New(5)))
	})
}

func TestMaroto_FitsNewPage(t *testing.T) {
	t.Run("when there is no header, should offer the whole page less the slack", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		sut.AddRow(19)

		assert.True(t, sut.FitsNewPage(row.New(19.9998)))
		assert.False(t, sut.FitsNewPage(row.New(20)))
	})
	t.Run("when a header and a footer are registered, should offer the space between them", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		_ = sut.RegisterHeader(row.New(5))
		_ = sut.RegisterFooter(row.New(3))
		sut.AddRow(10)

		assert.True(t, sut.FitsNewPage(row.New(11.9998)))
		assert.False(t, sut.FitsNewPage(row.New(12)))
	})
	t.Run("when the current page is fresh, should agree with Fits", func(t *testing.T) {
		sut := maroto.New(smallPageConfig())
		_ = sut.RegisterHeader(row.New(5))
		sut.AddRow(10)
		sut.AddPages(page.New())

		for _, height := range []float64{1, 14.9998, 15, 16} {
			r := row.New(height)
			assert.Equal(t, sut.FitsNewPage(r), sut.Fits(r), "height %v", height)
		}
	})
}

// TestMaroto_LongDocumentPagination adds random auto and fixed rows to long documents with a
// header, a footer and page numbers. Before each row it asks Fits and FitsNewPage, then checks
// that every row landed where the predicates said and that the generated PDF has exactly one
// physical page per logical page. Without the page slack, a page filled to its exact height could
// trip gofpdf's own page break and push every later page number onto the wrong page.
func TestMaroto_LongDocumentPagination(t *testing.T) {
	const headerRows = 1
	const footerRows = 1

	for seed := int64(1); seed <= 40; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			t.Parallel()

			// GetStructure and Generate both close the last page, so each gets its own document.
			sut, pages := buildLongDocument(t, seed)
			counts := rowCounts(sut.GetStructure())
			assert.Equal(t, len(pages), len(counts))
			for i := range counts {
				if i < len(pages) {
					// Each page also holds the header, the filler row and the footer.
					assert.Equal(t, pages[i], counts[i]-headerRows-1-footerRows, "page %d", i+1)
				}
			}

			sut, pages = buildLongDocument(t, seed)
			doc, err := sut.Generate()
			assert.Nil(t, err)
			physical, err := api.PageCount(bytes.NewReader(doc.GetBytes()), nil)
			assert.Nil(t, err)
			assert.Equal(t, len(pages), physical)
		})
	}
}

// buildLongDocument returns the document and the number of content rows Fits and FitsNewPage
// predicted for each page.
func buildLongDocument(t *testing.T, seed int64) (core.Maroto, []int) {
	rng := rand.New(rand.NewSource(seed))
	sut := maroto.New(config.NewBuilder().
		WithPageSize(pagesize.A4).
		WithTopMargin(15).
		WithLeftMargin(15).
		WithRightMargin(15).
		WithBottomMargin(10).
		WithPageNumber(props.PageNumber{Pattern: "Pag. {current}/{total}", Place: props.Bottom, Size: 9}).
		Build())
	_ = sut.RegisterHeader(row.New(30).Add(col.New(12)))
	_ = sut.RegisterFooter(row.New(12).Add(col.New(12)))

	pages := []int{0}
	for i := 0; i < 200+int(seed)*10; i++ {
		r := randomRow(rng, i)
		if sut.Fits(r) {
			pages[len(pages)-1]++
		} else {
			assert.True(t, sut.FitsNewPage(r), "row %d should fit on a new page", i)
			pages = append(pages, 1)
		}
		sut.AddRows(r)
	}

	return sut, pages
}

var concepts = strings.Fields("Cuota extraordinaria aprobada en asamblea para la impermeabilización de azoteas, reparación de elevadores y pintura de fachadas norte y sur, mantenimiento de áreas comunes")

func randomRow(rng *rand.Rand, i int) core.Row {
	// Fixed rows in multiples of 3.3867 mm, a table row height from a real document. Short
	// decimals like these let a page add up to its exact height.
	if rng.Intn(2) == 0 {
		return row.New(3.3867 * float64(1+rng.Intn(4))).Add(col.New(12))
	}

	words := concepts[:1+rng.Intn(len(concepts))]
	prop := props.Text{Size: 8, Top: 0.8, Bottom: 1, VerticalPadding: 0.7, Left: 1.5, Right: 1.5}
	return row.New().Add(
		text.NewCol(2, "01/10/2026", prop),
		text.NewCol(6, strings.Join(words, " "), prop),
		text.NewCol(2, fmt.Sprintf("ROW%05d", i), prop),
		text.NewCol(2, "$1,000.00", prop),
	)
}

// rowCounts returns the number of rows on each page of the structure.
func rowCounts(structure *node.Node[core.Structure]) []int {
	var counts []int
	for _, p := range structure.GetNexts() {
		counts = append(counts, len(p.GetNexts()))
	}

	return counts
}
