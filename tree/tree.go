package tree

import (
	"fmt"
	"log"
)

type AssignmentNode struct {
	parent   *AssignmentNode
	Name     string
	URL      string
	children []*AssignmentNode
}

func (n *AssignmentNode) AppendChild(c *AssignmentNode) {
	log.Default().Println(fmt.Sprintf("Appending child %s to parent %s", c.Name, n.Name))
	c.parent = n
	n.children = append(n.children, c)
}

func BuildAssignmentNode(parent *AssignmentNode, name string, url string) *AssignmentNode {
	log.Default().Println(fmt.Sprintf("Building node %s", name))
	node := &AssignmentNode{
		Name:   name,
		URL:    url,
		parent: parent,
	}
	return node
}
func BuildRootAssignmentNode(name string, url string) *AssignmentNode {
	return BuildAssignmentNode(nil, name, url)
}
