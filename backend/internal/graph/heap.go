package graph

// minHeap 面向 Dijkstra 的手写二叉最小堆。
//
// 未采用 container/heap 有两个原因：其一，标准库接口需要 interface{} 装箱与
// 反复的接口方法调用，在百万次比较的热路径上开销可观；其二，标准库不支持
// decrease-key，只能靠「同一节点多次入堆 + 出堆时丢弃陈旧项」来绕过，
// 会使堆规模膨胀到 O(E)。这里用 pos 索引直接支持 decrease-key，堆规模恒为 O(V)。
type minHeap struct {
	items []heapItem
	pos   map[NodeID]int // 节点 -> 在 items 中的下标
}

type heapItem struct {
	node NodeID
	dist float64
}

func newMinHeap(capacity int) *minHeap {
	if capacity < 0 {
		capacity = 0
	}
	return &minHeap{
		items: make([]heapItem, 0, capacity),
		pos:   make(map[NodeID]int, capacity),
	}
}

// Len 返回堆中元素个数。
func (h *minHeap) Len() int { return len(h.items) }

// Contains 报告节点当前是否在堆中。
func (h *minHeap) Contains(id NodeID) bool {
	_, ok := h.pos[id]
	return ok
}

// Push 插入节点；若节点已在堆中且新距离更小，则执行 decrease-key。
func (h *minHeap) Push(id NodeID, dist float64) {
	if i, ok := h.pos[id]; ok {
		if dist < h.items[i].dist {
			h.items[i].dist = dist
			h.up(i)
		}
		return
	}
	h.items = append(h.items, heapItem{node: id, dist: dist})
	i := len(h.items) - 1
	h.pos[id] = i
	h.up(i)
}

// Pop 弹出距离最小的节点。堆空时第三个返回值为 false。
func (h *minHeap) Pop() (NodeID, float64, bool) {
	if len(h.items) == 0 {
		return "", 0, false
	}
	top := h.items[0]
	last := len(h.items) - 1
	h.swap(0, last)
	h.items = h.items[:last]
	delete(h.pos, top.node)
	if last > 0 {
		h.down(0)
	}
	return top.node, top.dist, true
}

func (h *minHeap) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		h.swap(i, parent)
		i = parent
	}
}

func (h *minHeap) down(i int) {
	n := len(h.items)
	for {
		l, r := 2*i+1, 2*i+2
		smallest := i
		if l < n && h.less(l, smallest) {
			smallest = l
		}
		if r < n && h.less(r, smallest) {
			smallest = r
		}
		if smallest == i {
			return
		}
		h.swap(i, smallest)
		i = smallest
	}
}

// less 比较两个堆元素。距离相等时以节点 ID 为次级键，
// 保证等代价路径下的选择结果可复现，而不是随机漂移。
func (h *minHeap) less(i, j int) bool {
	if h.items[i].dist != h.items[j].dist {
		return h.items[i].dist < h.items[j].dist
	}
	return h.items[i].node < h.items[j].node
}

func (h *minHeap) swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.pos[h.items[i].node] = i
	h.pos[h.items[j].node] = j
}
