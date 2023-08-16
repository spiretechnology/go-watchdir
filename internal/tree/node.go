package tree

type NodeType uint8

const (
	NodeTypeFile NodeType = iota
	NodeTypeFolder
)

type Node struct {
	Name     string
	Type     NodeType
	Children map[string]*Node
}

func NewTree() *Node {
	return &Node{
		Type:     NodeTypeFolder,
		Children: make(map[string]*Node),
	}
}

func NewNode(name string, typ NodeType) *Node {
	node := &Node{
		Name: name,
		Type: typ,
	}
	if typ == NodeTypeFolder {
		node.Children = make(map[string]*Node)
	}
	return node
}
