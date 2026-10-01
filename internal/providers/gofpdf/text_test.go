package gofpdf_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vanclief/maroto/v2/internal/providers/gofpdf"
	"github.com/vanclief/maroto/v2/pkg/consts/breakline"
	"github.com/vanclief/maroto/v2/pkg/consts/fontfamily"
	"github.com/vanclief/maroto/v2/pkg/consts/fontstyle"
	"github.com/vanclief/maroto/v2/pkg/core/entity"
	"github.com/vanclief/maroto/v2/pkg/props"

	"github.com/vanclief/maroto/v2/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewText(t *testing.T) {
	text := gofpdf.NewText(mocks.NewFpdf(t), mocks.NewMath(t), mocks.NewFont(t))

	assert.NotNil(t, text)
	assert.Equal(t, fmt.Sprintf("%T", text), "*gofpdf.text")
}

func TestGetLinesheight(t *testing.T) {
	t.Run("when a text that occupies two lines is sent with EmptySpaceStrategy, should two is returned", func(t *testing.T) {
		textProp := &props.Text{}
		textProp.MakeValid(&props.Font{Family: fontfamily.Arial, Size: 10, Style: fontstyle.Normal})

		font := mocks.NewFont(t)
		font.EXPECT().SetFont(textProp.Family, textProp.Style, textProp.Size)

		pdf := mocks.NewFpdf(t)
		pdf.EXPECT().UnicodeTranslatorFromDescriptor("").Return(func(s string) string { return s })
		pdf.EXPECT().GetStringWidth("text").Return(5.0)  // First token just returns text
		pdf.EXPECT().GetStringWidth(" text").Return(6.0) // subsequent tokens return leading space

		text := gofpdf.NewText(pdf, mocks.NewMath(t), font)

		height := text.GetLinesQuantity("text text text text", textProp, 11)

		assert.Equal(t, 2, height)
	})

	t.Run("When a text that occupies two lines is sent with EmptySpaceStrategy, should two is returned", func(t *testing.T) {
		textProp := &props.Text{BreakLineStrategy: breakline.DashStrategy}
		textProp.MakeValid(&props.Font{Family: fontfamily.Arial, Size: 10, Style: fontstyle.Normal})

		font := mocks.NewFont(t)
		font.EXPECT().SetFont(textProp.Family, textProp.Style, textProp.Size)

		pdf := mocks.NewFpdf(t)
		pdf.EXPECT().GetStringWidth("t").Return(1)
		pdf.EXPECT().GetStringWidth(" ").Return(1)
		pdf.EXPECT().GetStringWidth(" - ").Return(1)
		pdf.EXPECT().UnicodeTranslatorFromDescriptor("").Return(func(s string) string { return s })

		text := gofpdf.NewText(pdf, mocks.NewMath(t), font)

		height := text.GetLinesQuantity("tttt tttt tttt tttt", textProp, 11)

		assert.Equal(t, 2, height)
	})
}


// byteWidth makes every byte one unit wide.
func byteWidth(s string) float64 {
	return float64(len(s))
}

func newCharacterBreakText(t *testing.T, strategy breakline.Strategy, translator func(string) string) (gofpdfText, *[]string) {
	textProp := &props.Text{BreakLineStrategy: strategy}
	textProp.MakeValid(&props.Font{Family: fontfamily.Arial, Size: 10, Style: fontstyle.Normal})

	font := mocks.NewFont(t)
	font.EXPECT().SetFont(textProp.Family, textProp.Style, textProp.Size)
	font.EXPECT().GetHeight(textProp.Family, textProp.Style, textProp.Size).Return(1).Maybe()
	font.EXPECT().GetColor().Return(&props.Color{}).Maybe()
	font.EXPECT().SetColor(mock.Anything).Maybe()

	drawn := []string{}
	pdf := mocks.NewFpdf(t)
	pdf.EXPECT().UnicodeTranslatorFromDescriptor("").Return(translator)
	pdf.EXPECT().GetStringWidth(mock.Anything).RunAndReturn(byteWidth)
	pdf.EXPECT().GetMargins().Return(0, 0, 0, 0).Maybe()
	pdf.EXPECT().Text(mock.Anything, mock.Anything, mock.Anything).Run(func(_ float64, _ float64, line string) {
		drawn = append(drawn, line)
	}).Maybe()

	return gofpdfText{gofpdf.NewText(pdf, mocks.NewMath(t), font), textProp}, &drawn
}

type gofpdfText struct {
	text interface {
		Add(text string, cell *entity.Cell, textProp *props.Text)
		GetLinesQuantity(text string, textProp *props.Text, colWidth float64) int
	}
	prop *props.Text
}

func TestText_CharacterBreak(t *testing.T) {
	identity := func(s string) string { return s }

	t.Run("when the strategy is EmptySpaceStrategy, should keep a word wider than the column whole", func(t *testing.T) {
		sut, drawn := newCharacterBreakText(t, breakline.EmptySpaceStrategy, identity)

		lines := sut.text.GetLinesQuantity("ab cdefghi j", sut.prop, 5)
		sut.text.Add("ab cdefghi j", &entity.Cell{Width: 5, Height: 10}, sut.prop)

		assert.Equal(t, 3, lines)
		assert.Equal(t, []string{"ab", "cdefghi", "j"}, *drawn)
	})

	t.Run("when the strategy is BreakWordStrategy and a word is wider than the column, should cut it where it stops fitting, without a dash", func(t *testing.T) {
		sut, drawn := newCharacterBreakText(t, breakline.BreakWordStrategy, identity)

		lines := sut.text.GetLinesQuantity("aaaaaaaaaa", sut.prop, 4)
		sut.text.Add("aaaaaaaaaa", &entity.Cell{Width: 4, Height: 10}, sut.prop)

		assert.Equal(t, 3, lines)
		assert.Equal(t, []string{"aaaa", "aaaa", "aa"}, *drawn)
	})
	t.Run("when the last piece of a cut word leaves room, should continue the line with the next word", func(t *testing.T) {
		sut, drawn := newCharacterBreakText(t, breakline.BreakWordStrategy, identity)

		lines := sut.text.GetLinesQuantity("ab cdefghi j", sut.prop, 5)
		sut.text.Add("ab cdefghi j", &entity.Cell{Width: 5, Height: 10}, sut.prop)

		assert.Equal(t, 3, lines)
		assert.Equal(t, []string{"ab", "cdefg", "hi j"}, *drawn)
	})
	t.Run("when the column is narrower than one character, should put one character per line", func(t *testing.T) {
		sut, drawn := newCharacterBreakText(t, breakline.BreakWordStrategy, identity)

		lines := sut.text.GetLinesQuantity("abc", sut.prop, 0.5)
		sut.text.Add("abc", &entity.Cell{Width: 0.5, Height: 10}, sut.prop)

		assert.Equal(t, 3, lines)
		assert.Equal(t, []string{"a", "b", "c"}, *drawn)
	})
	t.Run("when the text was translated to a single-byte encoding, should keep its bytes", func(t *testing.T) {
		// Built-in fonts draw cp1252, where «é» is the single byte 0xE9.
		cp1252 := func(s string) string { return strings.ReplaceAll(s, "é", "\xe9") }
		sut, drawn := newCharacterBreakText(t, breakline.BreakWordStrategy, cp1252)

		lines := sut.text.GetLinesQuantity("cafécafé", sut.prop, 4)
		sut.text.Add("cafécafé", &entity.Cell{Width: 4, Height: 10}, sut.prop)

		assert.Equal(t, 2, lines)
		assert.Equal(t, []string{"caf\xe9", "caf\xe9"}, *drawn)
	})
}
