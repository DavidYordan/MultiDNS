package utils

import (
	"bufio"
	"os"
	"strings"
)

type TrieNode struct {
	children map[string]*TrieNode
	isEnd    bool
}

func NewTrieNode() *TrieNode {
	return &TrieNode{children: make(map[string]*TrieNode)}
}

type Trie struct {
	root *TrieNode
}

func NewTrie() *Trie {
	return &Trie{root: NewTrieNode()}
}

func (t *Trie) Insert(domain string) {
	node := t.root
	parts := strings.Split(domain, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			continue
		}
		if _, exists := node.children[part]; !exists {
			node.children[part] = NewTrieNode()
		}
		node = node.children[part]
	}
	node.isEnd = true
}

func (t *Trie) Search(domain string) bool {
	parts := strings.Split(domain, ".")
	for i := 0; i < len(parts); i++ {
		subdomain := strings.Join(parts[i:], ".")
		if t.searchExact(subdomain) {
			return true
		}
	}
	return false
}

func (t *Trie) searchExact(domain string) bool {
	node := t.root
	parts := strings.Split(domain, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			continue
		}
		if _, exists := node.children[part]; !exists {
			return false
		}
		node = node.children[part]
		if node.isEnd {
			return true
		}
	}
	return node.isEnd
}

func LoadCNDomains(filePath string) (*Trie, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	trie := NewTrie()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		domain := scanner.Text()
		if domain != "" {
			trie.Insert(domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return trie, nil
}
