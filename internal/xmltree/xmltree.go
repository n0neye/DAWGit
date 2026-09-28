// Package xmltree is a minimal ordered XML tree with byte-exact round-tripping
// of Ableton Live Set XML.
//
// The model mirrors Python's ElementTree: character data before the first
// child is Text, character data after an element is that element's Tail.
package xmltree

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const xmlDecl = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

type Attr struct {
	Name  string
	Value string
}

type Node struct {
	Tag      string
	Attrs    []Attr
	Text     string
	Tail     string
	Children []*Node
}

// Parse reads an XML document and returns its root element.
func Parse(data []byte) (*Node, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var stack []*Node
	var root *Node
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{Tag: qname(t.Name)}
			for _, a := range t.Attr {
				n.Attrs = append(n.Attrs, Attr{qname(a.Name), a.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("xmltree: multiple root elements")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("xmltree: unexpected </%s>", qname(t.Name))
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue // whitespace outside the root element
			}
			s := strings.ReplaceAll(string(t), "\r\n", "\n")
			p := stack[len(stack)-1]
			if len(p.Children) == 0 {
				p.Text += s
			} else {
				p.Children[len(p.Children)-1].Tail += s
			}
		}
	}
	if root == nil {
		return nil, errors.New("xmltree: no root element")
	}
	return root, nil
}

func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// Serialize writes the document the way Live does: XML declaration, CRLF line
// endings, single-quoted attributes when the value contains a double quote.
func Serialize(root *Node) []byte {
	var b strings.Builder
	b.WriteString(xmlDecl)
	write(&b, root)
	b.WriteString("\n")
	return []byte(strings.ReplaceAll(b.String(), "\n", "\r\n"))
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func attrValue(v string) string {
	v = textEscaper.Replace(v)
	if strings.Contains(v, `"`) && !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	return `"` + strings.ReplaceAll(v, `"`, "&quot;") + `"`
}

func write(b *strings.Builder, n *Node) {
	b.WriteString("<")
	b.WriteString(n.Tag)
	for _, a := range n.Attrs {
		b.WriteString(" ")
		b.WriteString(a.Name)
		b.WriteString("=")
		b.WriteString(attrValue(a.Value))
	}
	if len(n.Children) == 0 && n.Text == "" {
		b.WriteString(" />")
	} else {
		b.WriteString(">")
		b.WriteString(textEscaper.Replace(n.Text))
		for _, c := range n.Children {
			write(b, c)
		}
		b.WriteString("</")
		b.WriteString(n.Tag)
		b.WriteString(">")
	}
	b.WriteString(textEscaper.Replace(n.Tail))
}

// --- attributes ---

func (n *Node) Get(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Attr returns the attribute value or "" when absent.
func (n *Node) Attr(name string) string {
	v, _ := n.Get(name)
	return v
}

func (n *Node) Has(name string) bool {
	_, ok := n.Get(name)
	return ok
}

func (n *Node) Set(name, value string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{name, value})
}

func (n *Node) DelAttr(name string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs = append(n.Attrs[:i], n.Attrs[i+1:]...)
			return
		}
	}
}

// --- navigation ---

// Find returns the first element matching a slash-separated path of child
// tags relative to n, or nil.
func (n *Node) Find(path string) *Node {
	cur := n
	for _, tag := range strings.Split(path, "/") {
		cur = cur.Child(tag)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// FindAll returns all elements matching a slash-separated path relative to n.
func (n *Node) FindAll(path string) []*Node {
	cur := []*Node{n}
	for _, tag := range strings.Split(path, "/") {
		var next []*Node
		for _, c := range cur {
			for _, cc := range c.Children {
				if cc.Tag == tag {
					next = append(next, cc)
				}
			}
		}
		cur = next
	}
	return cur
}

// Child returns the first direct child with the given tag, or nil.
func (n *Node) Child(tag string) *Node {
	for _, c := range n.Children {
		if c.Tag == tag {
			return c
		}
	}
	return nil
}

// Val returns the Value attribute of the element at path, or def.
func (n *Node) Val(path, def string) string {
	if n == nil {
		return def
	}
	e := n.Find(path)
	if e == nil {
		return def
	}
	if v, ok := e.Get("Value"); ok {
		return v
	}
	return def
}

// Walk visits n and all descendants in document order. Returning false from
// fn skips the node's children.
func (n *Node) Walk(fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Iter returns n and all descendants in document order whose tag matches
// (all nodes when tag is "").
func (n *Node) Iter(tag string) []*Node {
	var out []*Node
	n.Walk(func(e *Node) bool {
		if tag == "" || e.Tag == tag {
			out = append(out, e)
		}
		return true
	})
	return out
}

// --- mutation ---

func (n *Node) Clone() *Node {
	c := &Node{Tag: n.Tag, Text: n.Text, Tail: n.Tail}
	c.Attrs = append([]Attr(nil), n.Attrs...)
	c.Children = make([]*Node, len(n.Children))
	for i, ch := range n.Children {
		c.Children[i] = ch.Clone()
	}
	return c
}

func (n *Node) IndexOf(child *Node) int {
	for i, c := range n.Children {
		if c == child {
			return i
		}
	}
	return -1
}

func (n *Node) Insert(i int, child *Node) {
	n.Children = append(n.Children, nil)
	copy(n.Children[i+1:], n.Children[i:])
	n.Children[i] = child
}

func (n *Node) Append(child *Node) {
	n.Children = append(n.Children, child)
}

func (n *Node) Remove(child *Node) bool {
	i := n.IndexOf(child)
	if i < 0 {
		return false
	}
	n.Children = append(n.Children[:i], n.Children[i+1:]...)
	return true
}

// Replace swaps old for new at the same position, keeping old's tail.
func (n *Node) Replace(old, new *Node) bool {
	i := n.IndexOf(old)
	if i < 0 {
		return false
	}
	new.Tail = old.Tail
	n.Children[i] = new
	return true
}
