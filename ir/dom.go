package ir

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/kingfolk/capybara/types"
)

type DominatorMaker struct {
	debug      bool
	blockCount int
	rootBlock  *Block
	allBlocks  []*Block
	params     []string
	LiftParams []string
}

func NewDominatorMaker(root *Block, debug bool, params ...string) *DominatorMaker {
	visited := map[int]bool{}
	stack := []*Block{root}
	allBlocks := []*Block{}
	visited[root.Id] = true
	for len(stack) > 0 {
		top := stack[0]
		visited[top.Id] = true
		allBlocks = append(allBlocks, top)
		stack = stack[1:]
		for _, d := range top.Dest {
			if !visited[d.Id] {
				stack = append(stack, d)
				visited[d.Id] = true
			}
		}
	}

	maker := &DominatorMaker{
		debug:      debug,
		rootBlock:  root,
		blockCount: len(allBlocks),
		allBlocks:  allBlocks,
		params:     params,
	}
	return maker
}

type domInfo struct {
	idom      *Block
	children  []*Block
	pre, post int32
}

// ltState holds the working state for Lengauer-Tarjan algorithm
// (during which domInfo.pre is repurposed for CFG DFS preorder number).
type ltState struct {
	// Each slice is indexed by b.Id.
	sdom     []*Block // b's semidominator
	parent   []*Block // b's parent in DFS traversal of CFG
	ancestor []*Block // b's ancestor with least sdom
}

// dfs implements the depth-first search part of the LT algorithm.
func (lt *ltState) dfs(v *Block, i int32, preorder []*Block) int32 {
	preorder[i] = v
	v.dom.pre = i // For now: DFS preorder of spanning tree of CFG
	i++
	lt.sdom[v.Id] = v
	lt.link(nil, v)
	for _, w := range v.Dest {
		if lt.sdom[w.Id] == nil {
			lt.parent[w.Id] = v
			i = lt.dfs(w, i, preorder)
		}
	}
	return i
}

// eval implements the EVAL part of the LT algorithm.
func (lt *ltState) eval(v *Block) *Block {
	// TODO(adonovan): opt: do path compression per simple LT.
	u := v
	for ; lt.ancestor[v.Id] != nil; v = lt.ancestor[v.Id] {
		if lt.sdom[v.Id].dom.pre < lt.sdom[u.Id].dom.pre {
			u = v
		}
	}
	return u
}

// link implements the LINK part of the LT algorithm.
func (lt *ltState) link(v, w *Block) {
	lt.ancestor[w.Id] = v
}

func (m *DominatorMaker) AllBlocks() []*Block {
	return m.allBlocks
}

func (m *DominatorMaker) buildDomTree() {
	n := m.blockCount
	// Allocate space for 5 contiguous [n]*Block arrays:
	// sdom, parent, ancestor, preorder, buckets.
	space := make([]*Block, 5*n)
	lt := ltState{
		sdom:     space[0:n],
		parent:   space[n : 2*n],
		ancestor: space[2*n : 3*n],
	}

	// Step 1.  Number vertices by depth-first preorder.
	preorder := space[3*n : 4*n]
	root := m.rootBlock
	lt.dfs(root, 0, preorder)

	buckets := space[4*n : 5*n]
	copy(buckets, preorder)

	// In reverse preorder...
	for i := int32(n) - 1; i > 0; i-- {
		w := preorder[i]

		// Step 3. Implicitly define the immediate dominator of each node.
		for v := buckets[i]; v != w; v = buckets[v.dom.pre] {
			u := lt.eval(v)
			if lt.sdom[u.Id].dom.pre < i {
				v.dom.idom = u
			} else {
				v.dom.idom = w
			}
		}

		// Step 2. Compute the semidominators of all nodes.
		lt.sdom[w.Id] = lt.parent[w.Id]
		for _, v := range w.Src {
			u := lt.eval(v)
			if lt.sdom[u.Id].dom.pre < lt.sdom[w.Id].dom.pre {
				lt.sdom[w.Id] = lt.sdom[u.Id]
			}
		}

		lt.link(lt.parent[w.Id], w)

		if lt.parent[w.Id] == lt.sdom[w.Id] {
			w.dom.idom = lt.parent[w.Id]
		} else {
			buckets[i] = buckets[lt.sdom[w.Id].dom.pre]
			buckets[lt.sdom[w.Id].dom.pre] = w
		}
	}

	// The final 'Step 3' is now outside the loop.
	for v := buckets[0]; v != root; v = buckets[v.dom.pre] {
		v.dom.idom = root
	}

	// Step 4. Explicitly define the immediate dominator of each
	// node, in preorder.
	for _, w := range preorder[1:] {
		if w == root {
			w.dom.idom = nil
		} else {
			if w.dom.idom != lt.sdom[w.Id] {
				w.dom.idom = w.dom.idom.dom.idom
			}
			// Calculate Children relation as inverse of Idom.
			w.dom.idom.dom.children = append(w.dom.idom.dom.children, w)
		}
	}

	numberDomTree(root, 0, 0)
}

// numberDomTree sets the pre- and post-order numbers of a depth-first
// traversal of the dominator tree rooted at v.  These are used to
// answer dominance queries in constant time.
func numberDomTree(v *Block, pre, post int32) (int32, int32) {
	v.dom.pre = pre
	pre++
	for _, child := range v.dom.children {
		pre, post = numberDomTree(child, pre, post)
	}
	v.dom.post = post
	post++
	return pre, post
}

// domFrontier maps each block to the set of blocks in its dominance
// frontier.  The outer slice is conceptually a map keyed by
// Block.Index.  The inner slice is conceptually a set, possibly
// containing duplicates.
//
// TODO(adonovan): opt: measure impact of dups; consider a packed bit
// representation, e.g. big.Int, and bitwise parallel operations for
// the union step in the Children loop.
//
// domFrontier's methods mutate the slice's elements but not its
// length, so their receivers needn't be pointers.
type domFrontier [][]*Block

func (df domFrontier) add(u, v *Block) {
	p := &df[u.Id]
	for _, a := range *p {
		if a.Id == v.Id {
			return
		}
	}
	*p = append(*p, v)
}

// build builds the dominance frontier df for the dominator (sub)tree
// rooted at u, using the Cytron et al. algorithm.
//
// TODO(adonovan): opt: consider Berlin approach, computing pruned SSA
// by pruning the entire IDF computation, rather than merely pruning
// the DF -> IDF step.
func (df domFrontier) build(u *Block) {
	// Encounter each node u in postorder of dom tree.
	for _, child := range u.dom.children {
		df.build(child)
	}
	for _, vb := range u.Dest {
		if v := vb.dom; v.idom != u {
			df.add(u, vb)
		}
	}
	for _, w := range u.dom.children {
		for _, vb := range df[w.Id] {
			// TODO(adonovan): opt: use word-parallel bitwise union.
			if v := vb.dom; v.idom != u {
				df.add(u, vb)
			}
		}
	}
}

func (m *DominatorMaker) buildDomFrontier() domFrontier {
	df := make(domFrontier, m.blockCount)
	df.build(m.rootBlock)
	return df
}

func (m *DominatorMaker) placePhi(df domFrontier) {
	defsitesBySymbol := map[string][]int{}
	allocOrigs := map[int]map[string]bool{}
	allocPhis := map[int]map[string]bool{}
	for _, block := range m.allBlocks {
		for _, ir := range block.Ins {
			if arrayHasInt(defsitesBySymbol[ir.Ident], block.Id) {
				continue
			}
			defsitesBySymbol[ir.Ident] = append(defsitesBySymbol[ir.Ident], block.Id)
			if allocOrigs[block.Id] == nil {
				allocOrigs[block.Id] = map[string]bool{}
			}
			allocOrigs[block.Id][ir.Ident] = true
		}
	}

	// 得到allIdents且sort，用于后续的主循环放置的phi因为sort，顺序是确定的
	var allIdents []string
	for ident := range defsitesBySymbol {
		allIdents = append(allIdents, ident)
	}
	sort.Strings(allIdents)

	for _, allocSymbol := range allIdents {
		defsites := defsitesBySymbol[allocSymbol]
		w := append([]int{}, defsites...)
		for len(w) != 0 {
			n := w[0]
			w = w[1:]

			for _, y := range df[n] {
				if allocPhis[y.Id] == nil {
					allocPhis[y.Id] = map[string]bool{}
				}
				allocPhi := allocPhis[y.Id]
				if !allocPhi[allocSymbol] {
					allocPhi[allocSymbol] = true
					phiIr := &Instr{
						Ident: allocSymbol,
						Kind:  PhiKind,
						Val: &Phi{
							Orig:  allocSymbol,
							Edges: make([]string, len(y.Src)),
						},
					}
					var lastPhi int
					for idx, ir := range y.Ins {
						if ir.Kind == PhiKind {
							lastPhi = idx
							break
						}
					}
					irs := append([]*Instr{}, y.Ins[:lastPhi]...)
					irs = append(irs, phiIr)
					irs = append(irs, y.Ins[lastPhi:]...)
					y.Ins = irs

					// 如果y并没有定值allocSymbol，则将y加入到w中
					if !allocOrigs[y.Id][allocSymbol] {
						w = append(w, y.Id)
					}
				}
			}
		}
	}
}

type renamingStack struct {
	stack     map[string]int
	index     int
	origDecls map[string]types.ValType
	declTable map[string]types.ValType
}

func (r *renamingStack) copy() *renamingStack {
	stack := map[string]int{}
	for k, v := range r.stack {
		stack[k] = v
	}
	return &renamingStack{
		stack:     stack,
		index:     r.index,
		origDecls: r.origDecls,
		declTable: r.declTable,
	}
}

func (r *renamingStack) next() {
	r.index++
}

func (r *renamingStack) stackSymbol(symbol string) string {
	if IsDangle(symbol) {
		return symbol
	}
	if i, ok := r.stack[symbol]; ok {
		newIdent := GenVarIdent(i)
		r.declTable[newIdent] = r.origDecls[symbol]
		return newIdent
	}
	// 当symbol不为'$'开头时，则为global变量，不进行重命名。
	if symbol[0] == '$' {
		return DangleIdent()
	}
	return symbol
}

func GenVarIdent(id int) string {
	return "$v" + strconv.Itoa(id)
}

func DestructVarId(ident string) int {
	if ident[:2] != "$v" {
		panic("unreachable")
	}
	id, err := strconv.Atoi(ident[2:])
	if err != nil {
		// unreachable
		panic(err)
	}
	return id
}

var dangleIdent = "$v_dangle"

func IsDangle(ident string) bool {
	return ident == dangleIdent
}

func DangleIdent() string {
	return dangleIdent
}

func (r *renamingStack) push(symbol string) string {
	r.next()
	r.stack[symbol] = r.index
	return GenVarIdent(r.index)
}

func (m *DominatorMaker) renameBlock(block *Block, renaming *renamingStack) {
	irs := []*Instr{}
	var renameIr = func(ir *Instr) {
		if ir.Kind == PhiKind {
			renaming.push(ir.Ident)
			ir.Ident = renaming.stackSymbol(ir.Ident)
		} else {
			switch i := ir.Val.(type) {
			case *If:
				i.Cond = renaming.stackSymbol(i.Cond)
			case *Expr:
				for idx, arg := range i.Args {
					i.Args[idx] = renaming.stackSymbol(arg)
				}
			case *Ref:
				i.Ident = renaming.stackSymbol(i.Ident)
			case *Ret:
				i.Target = renaming.stackSymbol(i.Target)
			case *ArrGet:
				i.Arr = renaming.stackSymbol(i.Arr)
				i.Index = renaming.stackSymbol(i.Index)
			case *ArrPut:
				i.Arr = renaming.stackSymbol(i.Arr)
				i.Index = renaming.stackSymbol(i.Index)
				i.Right = renaming.stackSymbol(i.Right)
			case *StaticCall:
				for idx, arg := range i.Args {
					i.Args[idx] = renaming.stackSymbol(arg)
				}
			case *TraitCall:
				for idx, arg := range i.Args {
					i.Args[idx] = renaming.stackSymbol(arg)
				}
			case *RecLit:
				for idx, arg := range i.Args {
					i.Args[idx] = renaming.stackSymbol(arg)
				}
			case *RecAcs:
				i.Target = renaming.stackSymbol(i.Target)
			case *EnumVar:
				if i.Box != "" {
					i.Box = renaming.stackSymbol(i.Box)
				}
			case *Discriminant:
				i.Target = renaming.stackSymbol(i.Target)
			case *Box:
				i.Target = renaming.stackSymbol(i.Target)
			case *BoxTrait:
				i.Target = renaming.stackSymbol(i.Target)
			case *Unbox:
				i.Target = renaming.stackSymbol(i.Target)
			}
			renaming.push(ir.Ident)
			ir.Ident = renaming.stackSymbol(ir.Ident)
		}
	}

	for _, ir := range block.Ins {
		renameIr(ir)
		irs = append(irs, ir)
	}
	block.Ins = irs

	for _, dest := range block.Dest {
		ok := dest.Ins[0].Kind == PhiKind
		if !ok {
			continue
		}
		var predIdx int
		for i, s := range dest.Src {
			if s == block {
				predIdx = i
				break
			}
		}
		for _, ir := range dest.Ins {
			if ir.Kind != PhiKind {
				break
			}
			phi := ir.Val.(*Phi)
			phi.Tp = renaming.origDecls[phi.Orig]
			phi.Edges[predIdx] = renaming.stackSymbol(phi.Orig)
		}
	}

	for _, b := range block.dom.children {
		crenaming := renaming.copy()
		m.renameBlock(b, crenaming)
		renaming.index = crenaming.index
	}
}

func (m *DominatorMaker) rename(declTable map[string]types.ValType) map[string]types.ValType {
	renaming := &renamingStack{
		stack:     map[string]int{},
		origDecls: declTable,
		declTable: map[string]types.ValType{},
	}

	for _, paramIdent := range m.params {
		symbol := renaming.push(paramIdent)
		renaming.declTable[symbol] = declTable[paramIdent]
		m.LiftParams = append(m.LiftParams, symbol)
	}

	m.renameBlock(m.rootBlock, renaming)

	return renaming.declTable
}

// TODO 会出现 $v10 = PHI($v_dangle, $v4) 这样的指令。v_dangle实际上表示是一个空值，但是目前类型挺对于空值unit
// 并未完全将他与int等现有类型在类型系统上建立完整可推导的关系，且运行时也难以处理。所以这个PHI计算上是有困难的。
// 以下处理遇到这样的PHI，直接删去这个这个指令。这样的删去对目前的case可以pass
//
// 空值在现代语言里经常和some/none，match等语法连在一起处理。
func (m *DominatorMaker) removeIneffective() {
	for _, blk := range m.allBlocks {
		var newIns []*Instr
		for _, ins := range blk.Ins {
			left := ins.Ident
			switch v := ins.Val.(type) {
			case *Ref:
				right := v.Ident
				// 等号左右相同
				if left == right {
					continue
				}
			case *Phi:
				var effectCount int
				// var effectIdent string
				for _, edge := range v.Edges {
					if !IsDangle(edge) {
						effectCount++
						// effectIdent = edge
					}
				}
				if effectCount == 0 {
					continue
				}
				if effectCount == 1 {
					// ins.Val = NewRef(v.Tp, effectIdent)
					continue
				}
			}
			newIns = append(newIns, ins)
		}
		blk.Ins = newIns
	}
}

func (m *DominatorMaker) Lift(declTable map[string]types.ValType) map[string]types.ValType {
	m.buildDomTree()

	if m.debug {
		fmt.Println("--- dominator tree ---")
		for _, b := range m.allBlocks {
			fmt.Print(b.Id, ": ")
			for _, c := range b.dom.children {
				fmt.Print(c.Id, " ")
			}
			fmt.Println()
		}
	}

	df := m.buildDomFrontier()

	if m.debug {
		fmt.Println("--- dominator frontier ---")
		for i, d := range df {
			fmt.Print(i, ": ")
			for _, b := range d {
				fmt.Print(b.Id, " ")
			}
			fmt.Println("")
		}
	}

	m.placePhi(df)

	// if m.debug {
	// 	fmt.Println("--- after place phi ---")
	// 	rb := CFGString(m.rootBlock)
	// 	fmt.Println(rb)
	// 	fmt.Println("--- after place phi end ---")
	// }

	newDecls := m.rename(declTable)

	m.removeIneffective()

	// TODO removeDeadPhis

	return newDecls
}

func arrayHasInt(arr []int, v int) bool {
	for _, a := range arr {
		if a == v {
			return true
		}
	}
	return false
}
