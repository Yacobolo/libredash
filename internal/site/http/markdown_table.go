package http

import (
	"fmt"

	goldmarkast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	renderhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// semanticTableCellRenderer treats the first cell in each Markdown table body
// row as its row header. Documentation tables consistently use that column to
// name the option, field, status, or resource described by the remaining cells.
type semanticTableCellRenderer struct{}

func (semanticTableCellRenderer) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	registerer.Register(extensionast.KindTableCell, renderSemanticTableCell)
}

func renderSemanticTableCell(writer util.BufWriter, _ []byte, node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
	cell := node.(*extensionast.TableCell)
	isColumnHeader := cell.Parent().Kind() == extensionast.KindTableHeader
	isRowHeader := cell.Parent().Kind() == extensionast.KindTableRow && cell.PreviousSibling() == nil
	tag := "td"
	if isColumnHeader || isRowHeader {
		tag = "th"
	}

	if !entering {
		_, _ = fmt.Fprintf(writer, "</%s>\n", tag)
		return goldmarkast.WalkContinue, nil
	}

	_, _ = fmt.Fprintf(writer, "<%s", tag)
	if isColumnHeader {
		_, _ = writer.WriteString(` scope="col"`)
	} else if isRowHeader {
		_, _ = writer.WriteString(` scope="row"`)
	}
	if cell.Alignment != extensionast.AlignNone {
		_, _ = fmt.Fprintf(writer, ` style="text-align:%s"`, cell.Alignment.String())
	}
	if cell.Attributes() != nil {
		if tag == "th" {
			renderhtml.RenderAttributes(writer, cell, extension.TableThCellAttributeFilter)
		} else {
			renderhtml.RenderAttributes(writer, cell, extension.TableTdCellAttributeFilter)
		}
	}
	_ = writer.WriteByte('>')
	return goldmarkast.WalkContinue, nil
}
