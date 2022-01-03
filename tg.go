package tg

import (
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
)

/*
#cgo linux CFLAGS: -I/usr/include/tcl8.6
#cgo linux LDFLAGS: -ltcl8.6 -ltk8.6
#cgo windows CFLAGS: -IC:/Tcl/include/
#cgo windows LDFLAGS: C:/Tcl/bin/tcl86.dll C:/Tcl/bin/tk86.dll

#include <tk.h>

extern void cmdHandler(unsigned int cmdIndex, char* ss);
static inline int CmdCallback(ClientData clientData, Tcl_Interp *interp, int argc, CONST char *argv[]) {
   cmdHandler((unsigned int)clientData, Tcl_Merge(argc, argv));
   return 0;
}
static inline void RegisterCmd(Tcl_Interp *interp, char *cmdName, unsigned int cmdIndex) {
   Tcl_CreateCommand(interp, cmdName, CmdCallback, (void *)cmdIndex, (Tcl_CmdDeleteProc *)NULL );
}
*/
import "C"

// Flags for widgets
const (
	Normal          = 0         // default
	Expand          = 1 << iota // widget may expand if window resize if it's possible
	ScrollX                     // add horizontal scrollbar if it's possible for this widget
	ScrollY                     // add vertical scrollbar if it's possible for this widget
	Password                    // for Entry - show * instead of letter than typing
	Horizontal                  // for containers - use horizontal layout for adding widget (default layout is vertical)
	DoNotClose                  // for tabs in Notebook
	AskForClose                 // for tabs in Notebook
	NotebookBreakB1             // break <Button-1> for notebook
	NotModal                    // Dialog not modal
	Topmost                     // Dialog top most
)

var (
	interp       *C.Tcl_Interp
	callbackCmds []func(string)
	genNextId    func() string
	widgets      map[string]interface{}
	rt           *root
)

func tkstr(s string) string {
	return "\"" + s + "\""
}

func tklist(l []string) string {
	ls := " [list "
	for _, s := range l {
		ls += tkstr(s) + " "
	}
	ls += "]"
	return ls
}

func initIdGenerator() func() string {
	newId := 0
	f := func() string {
		newId++
		return strconv.Itoa(newId)
	}
	return f
}

//export cmdHandler
func cmdHandler(cmdIndex C.uint, ss *C.char) {
	callbackCmds[cmdIndex](C.GoString(ss))
}

func addCallbackCmd(command func(string)) string {
	name := genNextId()
	callbackCmds = append(callbackCmds, command)
	C.RegisterCmd(interp, C.CString(name), C.uint(len(callbackCmds)-1))
	return name
}

func SetVar(name string, val interface{}) {
	C.Tcl_SetVar(interp, C.CString(name), C.CString(fmt.Sprint(val)), 0)
}

func GetVar(name string) string {
	return C.GoString(C.Tcl_GetVar(interp, C.CString(name), 0))
}

func UnsetVar(name string) {
	C.Tcl_UnsetVar(interp, C.CString(name), 0)
}

func eval(script string) error {
	//fmt.Println(script)
	if C.Tcl_Eval(interp, C.CString(script)) != C.TCL_OK {
		err := errors.New(C.GoString(C.Tcl_GetStringResult(interp)))
		fmt.Println(err)
		return err
	}
	return nil
}

func result() string {
	return C.GoString(C.Tcl_GetStringResult(interp))
}

var table_sort = `proc sort_clients_table {tree col direction} {
    # Build something we can sort
    set data {}
    foreach row [$tree children {}] {
	lappend data [list [$tree set $row $col] $row]
    }

    set dir [expr {$direction ? "-decreasing" : "-increasing"}]
    set r -1

    # Now reshuffle the rows into the sorted order
    foreach info [lsort -dictionary -index 0 $dir $data] {
	$tree move [lindex $info 1] {} [incr r]
    }

    # Switch the heading so that it will sort in the opposite direction
    $tree heading $col -command\
				[list sort_clients_table $tree $col [expr {!$direction}]]
}`

// Initialise Tcl and Tk interpretators, Id generator, callback commands list.
func InitRoot(title string, flags uint) (Container, error) {
	interp = C.Tcl_CreateInterp()

	if C.Tcl_Init(interp) != C.TCL_OK {
		return nil, errors.New(C.GoString(C.Tcl_GetStringResult(interp)))
	}

	if C.Tk_Init(interp) != C.TCL_OK {
		return nil, errors.New(C.GoString(C.Tcl_GetStringResult(interp)))
	}

	callbackCmds = make([]func(string), 0)
	widgets = map[string]interface{}{}

	genNextId = initIdGenerator()

	//eval("package require Img")
	eval(table_sort)

	rt = newRoot(title, flags)

	return rt, nil
}

// Start main loop (last calling function).
func MainLoop() {
	C.Tk_MainLoop()
}

// Set ttk theme (available on Linux are "clam", "default", "classic", "alt").
func SetTheme(name string) error {
	if err := eval("ttk::setTheme " + name); err != nil {
		return err
	}
	return nil
}

func MessageBox(message string) {
	tkcmd := "tk_messageBox -message " + tkstr(message)
	eval(tkcmd)
}

//===============================
//-------     widgets     -------
//===============================
func WidgetById(id string) interface{} {
	return widgets[id]
}

//===== Component =====
type Component interface {
	create(parentId string) (string, uint)
}

type Container interface {
	Add(components ...Component)
}

//===== widget =====
type widget struct {
	id        string // this widget id in Tk form (".c1.c2")
	name      string // widget type in Tk (f.e. "ttk::label")
	initParam string // specific parameters for creating widget
	flags     uint   // Expand | ScrollX | ScrollY
}

func (w *widget) create(parentId string) (string, uint) {
	w.id = parentId + "." + genNextId()
	tkcmd := w.name + " " + w.id + " " + w.initParam
	eval(tkcmd)
	return w.id, w.flags
}

func (w *widget) Destroy() {
	tkcmd := "destroy " + w.id
	eval(tkcmd)
}

func (w *widget) DestroyChildren() {
	eval("winfo children " + w.id)
	for _, c := range strings.Split(result(), " ") {
		eval("destroy " + c)
	}
}

func (w *widget) Bind(event string, params string, cb func(string)) {
	if params != "" {
		p := strings.Split(params, "")
		params = " %" + strings.Join(p, " %")
	}
	tkcmd := "bind " + w.id + " " + event + " {" + addCallbackCmd(cb) + params + "}"
	eval(tkcmd)
}

func (w *widget) EventGenerate(event string) {
	eval("event generate " + w.id + " " + event)
}

func (w *widget) Id() string {
	return w.id
}

//=======================Containers=============================
// Root
type root struct {
	toolBar      *Box
	main         *Box
	statusBar    *Box
	statusLabels []*Label
}

func newRoot(title string, flags uint) *root {
	r := root{}
	r.statusLabels = []*Label{}
	r.toolBar = NewBox(Horizontal)
	r.toolBar.create("")
	tkcmd := "pack " + r.toolBar.id + " -padx 1 -pady 1 -fill x"
	eval(tkcmd)
	r.main = NewBox(flags)
	r.main.create("")
	tkcmd = "pack " + r.main.id + " -padx 1 -pady 1 -fill both -expand yes"
	eval(tkcmd)
	r.statusBar = NewBox(Horizontal)
	r.statusBar.create("")
	tkcmd = "pack " + r.statusBar.id + " -padx 1 -pady 1 -fill x"
	eval(tkcmd)
	eval("wm title . " + tkstr(title))
	return &r
}

func (r *root) Add(components ...Component) {
	r.main.Add(components...)
}

func SetWinTitle(title string) {
	eval("wm title . " + tkstr(title))
}

func SetWinSize(w, h int) {
	eval(fmt.Sprintf("wm geometry . %dx%d", w, h))
}

func AddStatusField() {
	l := NewLabel("", 0)
	rt.statusBar.Add(l)
	rt.statusLabels = append(rt.statusLabels, l)
}

func AddStatus(n int, s string) {
	rt.statusLabels[n].SetText(s)
}

func AddToolButton(b *Button) {
	rt.toolBar.Add(b)
}

// ======= Dialog ========
type Dialog struct {
	id    string
	title string
	Box
}

func NewDialog(title string, flags uint) *Dialog {

	box := NewBox(flags)
	id := "." + genNextId()

	eval("toplevel " + id)
	eval("wm title " + id + " " + tkstr(title))
	eval("wm geometry " + id + " +50+50")

	if flags&Topmost != 0 {
		eval("wm attributes " + id + " -topmost 1")
	}

	eval("wm protocol " + id + " WM_DELETE_WINDOW {pwd}")
	box.create(id)
	eval("pack " + box.id + " -expand yes -fill both")
	d := Dialog{id, title, *box}
	return &d
}

func (d *Dialog) Call() {

	eval("focus " + d.id)
	eval("tkwait visibility " + d.id)
	if d.flags&NotModal != 0 {
		return
	}
	eval("grab " + d.id)
	eval("tkwait window " + d.id)
}

func (d *Dialog) SetSize(w, h int) {
	//eval(d.id + " configure -width " + strconv.Itoa(width))
	eval("wm geometry " + d.id + " " + strconv.Itoa(w) + "x" + strconv.Itoa(h))
}

func (d *Dialog) Destroy() {
	if d.flags&NotModal != 0 {
		eval("wm withdraw " + d.id)
	} else {
		eval("destroy " + d.id)
	}
}

// ======= Box ===========
type Box struct {
	widget
}

func NewBox(flags uint) *Box {
	initParam := ""
	w := widget{"", "ttk::frame", initParam, flags}
	b := Box{w}
	return &b
}

func (b *Box) create(parentId string) (string, uint) {
	id, flags := b.widget.create(parentId)
	widgets[id] = b
	return id, flags
}

func (b *Box) Add(components ...Component) {
	for _, c := range components {
		newId, flags := c.create(b.id)
		tkcmd := "pack " + newId + " -padx 1 -pady 1"
		if b.flags&Horizontal != 0 {
			tkcmd += " -side left"
		}
		tkcmd += " -fill "
		expand := flags&Expand != 0
		if expand {
			tkcmd += "both -expand yes"
		} else {
			if b.flags&Horizontal != 0 {
				tkcmd += "y"
			} else {
				tkcmd += "x"
			}
		}
		eval(tkcmd)
	}
}

// ======= Splitter ===========
type Splitter struct {
	widget
}

func NewSplitter(flags uint) *Splitter {
	initParam := ""
	if flags&Horizontal != 0 {
		initParam += " -orient horizontal"
	}
	w := widget{"", "ttk::panedwindow", initParam, flags}
	s := Splitter{w}
	return &s
}

func (s *Splitter) create(parentId string) (string, uint) {
	id, flags := s.widget.create(parentId)
	widgets[id] = s
	return id, flags
}

func (s *Splitter) Add(components ...Component) {
	for _, c := range components {
		newId, flags := c.create(s.id)
		w := " -weight "
		expand := flags&Expand != 0
		if expand {
			w += "1"
		} else {
			w += "0"
		}

		eval(s.id + " add " + newId + w)

	}
}

// ======= Notebook Tab ===========
type Tab struct {
	Box
	title string
}

func (t *Tab) SetTitle(s string) {
	fmt.Println(s)
}

// ======= Notebook ===========
type Notebook struct {
	widget
	tabs map[string]*Tab
}

func NewNotebook(flags uint) *Notebook {
	initParam := ""
	w := widget{"", "ttk::notebook", initParam, flags}
	t := make(map[string]*Tab)
	n := Notebook{w, t}
	return &n
}

func (n *Notebook) create(parentId string) (string, uint) {
	id, flags := n.widget.create(parentId)
	widgets[id] = n

	if flags&NotebookBreakB1 != 0 {
		eval("bind " + n.id + " <Double-1> {break}")
		eval("bind " + n.id + " <Button-1> {break}")
	} else {

		n.Bind("<Double-1>", "", func(s string) {
			t, _ := n.GetSelection()
			tab := n.tabs[t]
			if tab.flags&DoNotClose == 0 {
				eval(n.id + " forget " + t)
			}

		})
	}
	return id, flags
}

func (n *Notebook) NewTab(title string, flags uint) *Tab {
	b := NewBox(flags)
	t := Tab{*b, title}
	newId, _ := t.create(n.id)
	n.tabs[newId] = &t
	eval(n.id + " add " + newId + " -text " + tkstr(t.title) + " -sticky news")
	return &t
}

func (n *Notebook) GetSelection() (string, string) {
	err := eval(n.id + " select")
	if err != nil {
		fmt.Println(err)
		return "", ""
	}
	sel := result()

	err = eval(n.id + " tab " + sel + " -text")
	if err != nil {
		fmt.Println(err)
		return sel, ""
	}
	val := result()
	return sel, val
}

func (n *Notebook) IfSelect(f func(sel, val string)) {
	n.Bind("<<NotebookTabChanged>>", "", func(s string) {
		sel, val := n.GetSelection()
		f(sel, val)
	})
}

func (n *Notebook) Select(tabId string) {
	eval(n.id + " select " + tabId)
}

func (n *Notebook) SelectTab(tab *Tab) {
	eval(n.id + " select " + tab.id)
}

// ======= Grid ===========
type Grid struct {
	widget
	columns int
	row     int
	col     int
}

func NewGrid(columns int, flags uint) *Grid {
	initParam := ""
	w := widget{"", "ttk::frame", initParam, flags}
	g := Grid{w, columns, 0, 0}
	return &g
}

func (g *Grid) create(parentId string) (string, uint) {
	id, flags := g.widget.create(parentId)
	widgets[id] = g
	return id, flags
}

func (g *Grid) Add(components ...Component) {
	for _, c := range components {
		if g.col >= g.columns {
			g.col = 0
			g.row += 1
		}
		if c != nil {
			newId, flags := c.create(g.id)
			expand := flags&Expand != 0
			tkcmd := fmt.Sprintf("grid %s -row %d -column %d -sticky news -padx 1 -pady 1",
				newId, g.row, g.col)
			eval(tkcmd)

			if expand {
				eval(fmt.Sprintf("grid rowconfigure %s %d -weight 1", g.id, g.row))
				eval(fmt.Sprintf("grid columnconfigure %s %d -weight 1", g.id, g.col))
			}

		}
		g.col += 1
	}
}

//=======================Widgets=============================

// =========================

// Label widget
type Label struct {
	widget
}

// Return pointer to new Label widget with text "text"
// Availabel flags Normal or Expand
func NewLabel(text string, flags uint) *Label {
	initParam := " -text " + tkstr(text)
	w := widget{"", "ttk::label", initParam, flags}
	l := Label{w}
	return &l
}

func (l *Label) create(parentId string) (string, uint) {
	id, flags := l.widget.create(parentId)
	widgets[id] = l
	return id, flags
}

func (l *Label) SetText(text string) {
	eval(l.id + " configure -text " + tkstr(text))
}

func (l *Label) Text() string {
	eval(l.id + " cget -text")
	return result()
}

func (l *Label) Color(fg string, bg string) {
	if fg != "" {
		eval(l.id + " configure -foreground " + fg)
	}
	if bg != "" {
		eval(l.id + " configure -background " + bg)
	}
}

// ======== Button =================
type Button struct {
	widget
}

func NewButton(text string, flags uint) *Button {
	initParam := " -text " + tkstr(text)
	w := widget{"", "ttk::button", initParam, flags}
	b := Button{w}
	return &b
}

func (b *Button) create(parentId string) (string, uint) {
	id, flags := b.widget.create(parentId)
	widgets[id] = b
	return id, flags
}

func (b *Button) SetText(text string) {
	eval(b.id + " configure -text " + tkstr(text))
}

func (b *Button) IfPressed(command func(string)) {
	eval(b.id + " configure -command " + addCallbackCmd(command))
}

func (b *Button) GetText() string {
	eval(b.id + " cget -text")
	return result()
}

func (b *Button) Invoke() {
	tkcmd := b.id + " invoke"
	eval(tkcmd)
}

func (b *Button) Width(width int) {
	eval(b.id + " configure -width " + strconv.Itoa(width))
}

// Check widget
type Check struct {
	widget
	state    bool
	variable string
}

// Return pointer to new Check widget with text "text"
// Availabel flags Normal or Expand
func NewCheck(text string, defstate bool, flags uint) *Check {
	initParam := " -text " + tkstr(text)
	w := widget{"", "ttk::checkbutton", initParam, flags}
	return &Check{w, defstate, ""}
}

func (w *Check) create(parentId string) (string, uint) {
	id, flags := w.widget.create(parentId)
	widgets[id] = w
	w.variable = genNextId()
	if w.state {
		SetVar(w.variable, "1")
	} else {
		SetVar(w.variable, "0")
	}
	eval(id + " configure -variable " + w.variable)
	return id, flags
}

func (w *Check) Get() bool {
	res := GetVar(w.variable)
	if res == "0" {
		return false
	}
	return true
}

func (b *Check) IfCheck(command func(string)) {
	eval(b.id + " configure -command " + addCallbackCmd(command))
}

// ======== Entry =================
type Entry struct {
	widget
	text string
}

func NewEntry(text string, flags uint) *Entry {
	initParam := ""
	if flags&Password != 0 {
		initParam += "-show *"
	}
	w := widget{"", "ttk::entry", initParam, flags}
	e := Entry{w, text}
	return &e
}

func (e *Entry) create(parentId string) (string, uint) {
	id, flags := e.widget.create(parentId)
	widgets[id] = e
	eval(id + " insert 0 " + tkstr(e.text))
	return id, flags
}

//func (e *Entry) ById(id string) {}

func (e *Entry) Clear() {
	eval(e.id + " delete 0 end")
}

func (e *Entry) SetText(text string) {
	e.Clear()
	eval(e.id + " insert 0 " + tkstr(text))
}

func (e *Entry) GetText() string {
	eval(e.id + " get")
	return result()
}

func (e *Entry) GetDotText() string {
	return strings.Replace(e.GetText(), ",", ".", 1)
}

func (e *Entry) IfPressEnter(f func(str string)) {
	e.Bind("<Key-Return>", "", func(s string) {
		str := e.GetText()
		f(str)
	})
}

func IndexOfValueInSlice(data []string, val string) int {
	for k, v := range data {
		if v == val {
			return k
		}
	}
	return -1
}

// ======== Listbox =================
type Listbox struct {
	widget
	b       *Box
	list    []string
	listvar string
}

func NewListbox(list []string, flags uint) *Listbox {
	lv := genNextId()
	initParam := " -listvariable " + lv
	w := widget{"", "listbox", initParam, flags}
	b := NewBox(flags & Expand)
	l := Listbox{w, b, list, lv}
	return &l
}

func (l *Listbox) create(parentId string) (string, uint) {

	id, flags := l.b.widget.create(parentId)
	widgets[id] = l
	l.id = l.b.id + "." + genNextId()
	tkcmd := l.name + " " + l.id + " " + l.initParam
	eval(tkcmd)
	ys := ""
	if l.flags&ScrollY != 0 {
		ys = l.b.id + "." + genNextId()
		tkcmd = l.id + " configure -yscrollcommand \"" + ys + " set\""
		eval(tkcmd)
		tkcmd = "ttk::scrollbar " + ys + " -command \"" + l.id + " yview\" -orient vertical"
		eval(tkcmd)
	}
	tkcmd = "grid " + l.id + " " + ys + " -sticky news"
	eval(tkcmd)

	if l.flags&ScrollX != 0 {
		xs := l.b.id + "." + genNextId()

		tkcmd = "ttk::scrollbar " + xs + " -command \"" + l.id + " xview\" -orient horizontal"
		eval(tkcmd)
		tkcmd = l.id + " configure -xscrollcommand \"" + xs + " set\""
		eval(tkcmd)
		tkcmd = "grid " + xs + " -sticky ew"
		eval(tkcmd)
	}

	tkcmd = "grid rowconfigure " + l.b.id + " 0 -weight 1"
	eval(tkcmd)
	tkcmd = "grid columnconfigure " + l.b.id + " 0 -weight 1"
	eval(tkcmd)
	l.ListToBox()
	return id, flags
}

func (l *Listbox) Clear() {
	eval(l.id + " delete 0 end")
}

func (l *Listbox) List() []string {
	return l.list
}

func (l *Listbox) UpdateWithList(list []string) {
	eval("set " + l.listvar + tklist(list))
}

func (l *Listbox) ListToBox() {
	eval("set " + l.listvar + tklist(l.list))
}

func (l *Listbox) UpdateList(list []string) {
	l.list = list
}

func (l *Listbox) GetSelection() (int, string) {
	err := eval(l.id + " curselection")
	if err != nil {
		fmt.Println(err)
		return -1, ""
	}
	sel := result()
	if sel == "" {
		return -1, ""
	}
	err = eval(l.id + " get " + sel)
	if err != nil {
		fmt.Println(err)
		return -1, ""
	}
	val := result()
	n, _ := strconv.Atoi(sel)
	return n, val
}

func (l *Listbox) IfSelect(f func(index int, val string)) {
	l.Bind("<<ListboxSelect>>", "", func(s string) {
		index, val := l.GetSelection()
		if index != -1 {
			f(index, val)
		}
	})
}

func (l *Listbox) SetSelection(i int) {
	index := strconv.Itoa(i)
	eval(l.id + " activate " + index)
	eval(l.id + " selection set " + index)
	eval(l.id + " see " + index)
	eval("event generate " + l.id + " <<ListboxSelect>>")
}

func (l *Listbox) SetSelectionValue(s string) {
	i := IndexOfValueInSlice(l.list, s)
	l.SetSelection(i)
}

// ======== Tree =================
type Tree struct {
	widget
	b *Box
}

func NewTree(flags uint) *Tree {
	initParam := " -show tree"

	w := widget{"", "ttk::treeview", initParam, flags}
	b := NewBox(flags | Expand)
	t := Tree{w, b}
	return &t
}

func (t *Tree) create(parentId string) (string, uint) {

	id, flags := t.b.widget.create(parentId)
	widgets[id] = t
	t.id = t.b.id + "." + genNextId()
	tkcmd := t.name + " " + t.id + " " + t.initParam
	eval(tkcmd)
	ys := ""
	if t.flags&ScrollY != 0 {
		ys = t.b.id + "." + genNextId()
		tkcmd = t.id + " configure -yscrollcommand \"" + ys + " set\""
		eval(tkcmd)
		tkcmd = "ttk::scrollbar " + ys + " -command \"" + t.id + " yview\" -orient vertical"
		eval(tkcmd)
	}
	tkcmd = "grid " + t.id + " " + ys + " -sticky news"
	eval(tkcmd)

	if t.flags&ScrollX != 0 {
		xs := t.b.id + "." + genNextId()

		tkcmd = "ttk::scrollbar " + xs + " -command \"" + t.id + " xview\" -orient horizontal"
		eval(tkcmd)
		tkcmd = t.id + " configure -xscrollcommand \"" + xs + " set\""
		eval(tkcmd)
		tkcmd = "grid " + xs + " -sticky ew"
		eval(tkcmd)
	}

	tkcmd = "grid rowconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)
	tkcmd = "grid columnconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)

	return id, flags
}

func (t *Tree) GetSelection() (string, string) {
	err := eval(t.id + " selection")
	if err != nil {
		fmt.Println(err)
		return "", ""
	}
	sel := result()
	if sel == "" {
		return "", ""
	}

	err = eval(t.id + " item " + sel + " -values")
	if err != nil {
		fmt.Println(err)
		return sel, ""
	}
	val := result()

	return sel, val
}

func (t *Tree) IfSelect(f func(sel string, val string)) {
	t.Bind("<<TreeviewSelect>>", "", func(s string) {
		sel, val := t.GetSelection()
		f(sel, val)
	})
}

func (t *Tree) Columns(columns string) {
	eval(t.id + " configure -columns " + tkstr(columns))
}

// .2.tr insert 1 end -id 3 -text ccc -values [list sss 222 333]
func (t *Tree) Insert(parent string, id string, item string, data []string) {
	if data != nil {
		eval(t.id + " insert " + parent + " end -id " + id + " -text " + tkstr(item) + " -values " + tklist(data) + " -open true")
	} else {
		eval(t.id + " insert " + parent + " end -id " + id + " -text " + tkstr(item) + " -open true")
	}
}

// insert not open
func (t *Tree) InsertNO(parent string, id string, item string, data []string) {
	if data != nil {
		eval(t.id + " insert " + parent + " end -id " + id + " -text " + tkstr(item) + " -values " + tklist(data))
	} else {
		eval(t.id + " insert " + parent + " end -id " + id + " -text " + tkstr(item))
	}
}

func (t *Tree) InsertData(parent string, data []string) {
	eval(t.id + " insert " + parent + " end -values " + tklist(data))
}

func (t *Tree) Clear() {
	eval(t.id + " delete [" + t.id + " children {}]")
}

func (t *Tree) SetSelection(s string) error {
	err1 := eval(t.id + " selection set [list " + s + "]")
	err2 := eval(t.id + " see " + tkstr(s))
	if err1 != nil || err2 != nil {
		return errors.New("Can't select item " + s + "!")
	}
	return nil
}

func (t *Tree) SetWidth(width int) {
	eval(t.id + " column #0 -width " + strconv.Itoa(width))
}

// ======== Image =================
type Image struct {
	widget
	imageFile string
}

func NewImage(imageFile string, flags uint) *Image {
	initParam := ""
	w := widget{"", "label", initParam, flags}
	i := Image{w, imageFile}
	return &i
}

func (i *Image) create(parentId string) (string, uint) {
	id, flags := i.widget.create(parentId)
	widgets[id] = i
	eval(id + " configure -image " + tkstr(i.imageFile))
	//	eval(id + " configure -image [image create photo -file " + tkstr(i.imageFile) + "]")
	/*
		bs, err := ioutil.ReadFile(i.imageFile)
		if err != nil {
			fmt.Println(err)
		}
		sEnc := b64.StdEncoding.EncodeToString(bs)
		eval(id + " configure -image [image create photo -data " + sEnc + "]")
	*/
	return id, flags
}

// ======== Combobox =================
type Combobox struct {
	widget
	list []string
}

func NewCombobox(list []string, flags uint) *Combobox {
	initParam := " -state readonly "
	if len(list) > 0 {
		initParam += " -values " + tklist(list)
	}
	w := widget{"", "ttk::combobox", initParam, flags}
	c := Combobox{w, list}
	return &c
}

func (c *Combobox) create(parentId string) (string, uint) {
	id, flags := c.widget.create(parentId)
	widgets[id] = c
	c.id = id
	if len(c.list) > 0 {
		eval(id + " current 0")
	}
	return id, flags
}

func (c *Combobox) Update(list []string) {
	eval(c.id + " configure -values " + tklist(list))
	eval(c.id + " current 0")
}

func (c *Combobox) SetSelection(i int) {
	eval(c.id + " current " + strconv.Itoa(i))
	eval("event generate " + c.id + " <<ComboboxSelected>>")
}

func (c *Combobox) SetValue(s string) {
	eval(c.id + " set " + tkstr(s))
	eval("event generate " + c.id + " <<ComboboxSelected>>")
}

func (c *Combobox) GetSelection() (int, string) {
	err := eval(c.id + " current")
	if err != nil {
		fmt.Println("No current in combobox")
		fmt.Println(err)
		return -1, ""
	}
	sel := result()
	if sel == "" {
		fmt.Println("No result in combobox")
		return -1, ""
	}
	err = eval(c.id + " get")
	if err != nil {
		fmt.Println("No get in combobox")
		fmt.Println(err)
		return -1, ""
	}
	val := result()
	n, _ := strconv.Atoi(sel)
	return n, val
}

func (c *Combobox) IfSelect(f func(index int, val string)) {
	c.Bind("<<ComboboxSelected>>", "", func(s string) {
		index, val := c.GetSelection()
		if index != -1 {
			f(index, val)
		}
	})
}

func (c *Combobox) SetWidth(width int) {
	eval(c.id + " configure -width " + strconv.Itoa(width))
}

// ======== Table =================
type Table struct {
	widget
	b    *Box
	data [][]string
}

func NewTable(data [][]string, flags uint) *Table {
	initParam := " -displaycolumns #all -show headings"
	w := widget{"", "ttk::treeview", initParam, flags}
	b := NewBox(flags & Expand)
	t := Table{w, b, data}
	return &t
}

func (t *Table) create(parentId string) (string, uint) {

	id, flags := t.b.widget.create(parentId)
	widgets[id] = t
	t.id = t.b.id + "." + genNextId()
	tkcmd := t.name + " " + t.id + " " + t.initParam
	eval(tkcmd)
	ys := ""
	if t.flags&ScrollY != 0 {
		ys = t.b.id + "." + genNextId()
		tkcmd = t.id + " configure -yscrollcommand \"" + ys + " set\""
		eval(tkcmd)
		tkcmd = "ttk::scrollbar " + ys + " -command \"" + t.id + " yview\" -orient vertical"
		eval(tkcmd)
	}
	tkcmd = "grid " + t.id + " " + ys + " -sticky news"
	eval(tkcmd)

	if t.flags&ScrollX != 0 {
		xs := t.b.id + "." + genNextId()

		tkcmd = "ttk::scrollbar " + xs + " -command \"" + t.id + " xview\" -orient horizontal"
		eval(tkcmd)
		tkcmd = t.id + " configure -xscrollcommand \"" + xs + " set\""
		eval(tkcmd)
		tkcmd = "grid " + xs + " -sticky ew"
		eval(tkcmd)
	}

	tkcmd = "grid rowconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)
	tkcmd = "grid columnconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)

	for _, i := range t.data {
		//eval(t.id + " insert {} end -text " + tkstr(i[0]) + " -values " + tklist(i))
		eval(t.id + " insert {} end -id " + i[len(i)-1] + " -text " + tkstr(i[0]) + " -values " + tklist(i))
	}

	return id, flags
}

func (t *Table) UpdateWithData(data [][]string) {
	for _, i := range data {
		//eval(t.id + " insert {} end -text " + tkstr(i[0]) + " -values " + tklist(i))
		eval(t.id + " insert {} end -id " + i[len(i)-1] + " -text " + tkstr(i[0]) + " -values " + tklist(i))
	}
}

func (t *Table) GetSelection() (string, string) {
	err := eval(t.id + " selection")
	if err != nil {
		fmt.Println(err)
		return "", ""
	}
	sel := result()
	if sel == "" {
		return "", ""
	}
	eval(t.id + " item " + sel + " -values")
	val := result()
	return sel, val
}

func (t *Table) IfSelect(f func(sel, val string)) {
	t.Bind("<<TreeviewSelect>>", "", func(s string) {
		sel, val := t.GetSelection()
		f(sel, val)
	})
}

func (t *Table) SetSelection(s string) {
	eval(t.id + " selection set [list " + s + "]")
	eval(t.id + " see " + tkstr(s))
}

func (t *Table) Columns(columns string) {
	eval(t.id + " configure -columns " + tkstr(columns))
	firstdata := t.data[0]
	lastdata := t.data[len(t.data)-1]

	for i, column := range strings.Split(columns, " ") {
		eval(t.id + " heading " + column + " -text " + column + " -command " + tkstr("sort_clients_table "+t.id+" "+column+" 1"))
		width := (len(firstdata[i]) + len(lastdata[i]) + len(column)) * 5
		eval(t.id + " column " + column + " -width " + strconv.Itoa(width))
	}

}

func (t *Table) SetColumnsWithWidth(columns string, width []int) {
	eval(t.id + " configure -columns " + tkstr(columns))
	for i, column := range strings.Split(columns, " ") {
		eval(t.id + " heading " + column + " -text " + column + " -command " + tkstr("sort_clients_table "+t.id+" "+column+" 1"))
		eval(t.id + " column " + column + " -width " + strconv.Itoa(width[i]))
	}
}

func (t *Table) SetColumns(columns string) {
	eval(t.id + " configure -columns " + tkstr(columns))
	for _, column := range strings.Split(columns, " ") {
		eval(t.id + " heading " + column + " -text " + column + " -command " + tkstr("sort_clients_table "+t.id+" "+column+" 1"))
	}
}

func (t *Table) Clear() {
	eval(t.id + " delete [" + t.id + " children {}]")
}

func (t *Table) Get() []string {
	res := []string{}
	eval(t.id + " children {}")
	rr := result()
	if rr == "" {
		return res
	}
	r := strings.Split(rr, " ")
	for _, v := range r {
		eval(t.id + " item " + v + " -values")
		res = append(res, result())
	}
	return res
}

func (t *Table) Delete(id string) {
	eval(t.id + " delete " + id)
}

func (t *Table) Append(i []string) {
	eval(t.id + " insert {} end -id " + i[len(i)-1] + " -text " + tkstr(i[0]) + " -values " + tklist(i))
}

/*
func (t *Tree) Insert(parent string, item string) {
	eval(t.id + " insert " + parent + " end -text " + tkstr(item))
}

func (t *Tree) InsertData(parent string, data []string) {
	eval(t.id + " insert " + parent + " end -values " + tklist(data))
}


*/

// ======== Text =================
type Text struct {
	widget
	b    *Box
	text string
}

func NewText(text string, flags uint) *Text {
	initParam := " -wrap word -width 20 -height 5"
	w := widget{"", "text", initParam, flags}
	b := NewBox(flags | Expand)
	t := Text{w, b, text}
	return &t
}

func (t *Text) create(parentId string) (string, uint) {

	id, flags := t.b.widget.create(parentId)
	widgets[id] = t
	t.id = t.b.id + "." + genNextId()
	tkcmd := t.name + " " + t.id + " " + t.initParam
	eval(tkcmd)
	ys := ""
	if t.flags&ScrollY != 0 {
		ys = t.b.id + "." + genNextId()
		tkcmd = t.id + " configure -yscrollcommand \"" + ys + " set\""
		eval(tkcmd)
		tkcmd = "ttk::scrollbar " + ys + " -command \"" + t.id + " yview\" -orient vertical"
		eval(tkcmd)
	}
	tkcmd = "grid " + t.id + " " + ys + " -sticky news"
	eval(tkcmd)

	if t.flags&ScrollX != 0 {
		xs := t.b.id + "." + genNextId()

		tkcmd = "ttk::scrollbar " + xs + " -command \"" + t.id + " xview\" -orient horizontal"
		eval(tkcmd)
		tkcmd = t.id + " configure -xscrollcommand \"" + xs + " set\""
		eval(tkcmd)
		tkcmd = "grid " + xs + " -sticky ew"
		eval(tkcmd)
	}

	tkcmd = "grid rowconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)
	tkcmd = "grid columnconfigure " + t.b.id + " 0 -weight 1"
	eval(tkcmd)

	eval(t.id + " insert 1.0 " + tkstr(t.text))
	return id, flags
}

func (t *Text) Clear() {
	eval(t.id + " delete 1.0 end")
}

func (t *Text) Insert(text string) {
	eval(t.id + " insert 1.0 " + tkstr(text))
}

func (t *Text) Add(text string) {
	eval(t.id + " insert end " + tkstr(text))
}

func (t *Text) Get() string {
	eval(t.id + " get 1.0 end")
	return strings.TrimSpace(result())
}

// ======== Calendar =================
type Calendar struct {
	Box
	lb            *Label
	bt            *Button
	ltextvariable string
	selWidgetId   string
	selWidgetBg   string
	lbtime        *Label
	dates         []*Label
	ltime         time.Time
	day           int
	tl            *Dialog
}

func NewCalendar(flags uint) *Calendar {
	//	initParam := ""

	b := NewBox(flags | Horizontal)
	t := Calendar{*b, NewLabel("", 0), NewButton(">", 0),
		"", "", "", NewLabel("", 0), []*Label{}, time.Time{}, 0, nil,
	}
	return &t
}

func (t *Calendar) create(parentId string) (string, uint) {
	id, flags := t.widget.create(parentId)
	widgets[id] = t
	t.Add(t.lb, t.bt)
	tm := time.Now()
	ltime := tm.Format("02.01.2006")
	t.ltextvariable = genNextId()
	SetVar(t.ltextvariable, ltime)
	eval(t.lb.id + " configure -textvariable " + t.ltextvariable)
	t.bt.IfPressed(t.call)
	return id, flags
}

func (t *Calendar) Set(s string) {
	t.lb.SetText(s)
	t.ltime, _ = time.Parse("02.01.2006", s)
}

func (t *Calendar) call(s string) {
	t.ltime, _ = time.Parse("02.01.2006", t.lb.Text())
	if len(t.dates) < 1 {

		//t.dates = []*Label{}
		t.tl = NewDialog("Оберіть дату", NotModal)
		bx := NewBox(Horizontal)
		grid := NewGrid(7, Expand)
		bbx := NewBox(Horizontal)
		t.tl.Add(t.lbtime, bx, grid, bbx)

		bDecYear := NewButton("<<", 0)
		bDecMonth := NewButton("<", 0)
		bIncMonth := NewButton(">", 0)
		bIncYear := NewButton(">>", 0)

		bx.Add(bDecYear, bDecMonth, bIncMonth, bIncYear)

		bDecYear.Width(3)
		bDecMonth.Width(2)
		bIncMonth.Width(2)
		bIncYear.Width(3)

		bDecYear.IfPressed(func(s string) {
			year, month, day := t.ltime.Date()
			t.ltime = time.Date(year-1, month, day, 0, 0, 0, 0, time.Local)
			//t.ltime = t.ltime.Add(-time.Hour * 365 * 24)
			t.makeMonth()
		})
		bDecMonth.IfPressed(func(s string) {
			year, month, day := t.ltime.Date()
			month = time.Month(int(month) - 1)
			t.ltime = time.Date(year, month, day, 0, 0, 0, 0, time.Local)
			//t.ltime = t.ltime.Add(-time.Hour * 31 * 24)
			t.makeMonth()
		})
		bIncMonth.IfPressed(func(s string) {
			year, month, day := t.ltime.Date()
			month = time.Month(int(month) + 1)
			t.ltime = time.Date(year, month, day, 0, 0, 0, 0, time.Local)
			//t.ltime = t.ltime.Add(time.Hour * 31 * 24)
			t.makeMonth()
		})
		bIncYear.IfPressed(func(s string) {
			year, month, day := t.ltime.Date()
			t.ltime = time.Date(year+1, month, day, 0, 0, 0, 0, time.Local)
			//t.ltime = t.ltime.Add(time.Hour * 365 * 24)
			t.makeMonth()
		})

		for _, v := range []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Нд"} {
			grid.Add(NewLabel(v, 0))
		}
		for i := 0; i < 42; i++ {
			l := NewLabel(" ", 0)
			grid.Add(l)
			l.Color("black", "gray")
			eval(l.id + " configure -anchor center")
			l.Bind("<Button-1>", "W", t.makeSelect)
			t.dates = append(t.dates, l)
		}

		bc := NewButton("Відмінити", 0)
		bs := NewButton("Прийняти", 0)
		bbx.Add(bc, bs)
		bc.Width(-1)
		bs.Width(-1)
		bc.IfPressed(func(s string) {
			t.tl.Destroy()
		})
		bs.IfPressed(func(s string) {
			year, month, _ := t.ltime.Date()
			dt := time.Date(year, month, t.day, 0, 0, 0, 0, time.Local)
			SetVar(t.ltextvariable, dt.Format("02.01.2006"))
			t.tl.Destroy()
		})
	} else {
		eval("wm deiconify " + t.tl.id)
	}
	t.makeMonth()
	t.tl.Call()
}

func (t *Calendar) makeMonth() {
	t.lbtime.SetText(t.ltime.Format("01.2006"))
	year, month, day := t.ltime.Date()
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	wday := start.Weekday()
	thisMonth := start.Month()
	for i := 0; i < 42; i++ {
		if i < 7 && i < int(wday)-1 {
			t.dates[i].SetText(" ")
			t.dates[i].Color("", "gray")
			continue
		}
		if thisMonth == start.Month() {
			t.dates[i].SetText(strconv.Itoa(start.Day()))
			t.dates[i].Color("", "gray")
			if day == start.Day() {
				t.dates[i].Color("", "yellow")
			}
			start = start.Add(time.Hour * 24)
			continue
		}
		t.dates[i].SetText(" ")
	}

}

func (t *Calendar) makeSelect(s string) {
	s = strings.Split(s, " ")[1]
	if t.selWidgetId != "" {
		eval(t.selWidgetId + " configure -background " + t.selWidgetBg)
	}
	t.selWidgetId = s
	eval(s + " cget -background")
	t.selWidgetBg = result()
	eval(s + " configure -background red")
	eval(s + " cget -text")
	t.day, _ = strconv.Atoi(result())
}

func (t *Calendar) GetText() string {
	return t.lb.Text()
}

func (t *Calendar) Today() {
	dt := time.Now()
	SetVar(t.ltextvariable, dt.Format("02.01.2006"))
}

func Upload_image(name string, img image.Image) error {
	//var buf bytes.Buffer
	//err := sprintf(&buf, "image create photo %{}", name)

	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		// let's do it slowpoke
		bounds := img.Bounds()
		nrgba = image.NewNRGBA(bounds)
		for x := 0; x < bounds.Max.X; x++ {
			for y := 0; y < bounds.Max.Y; y++ {
				nrgba.Set(x, y, img.At(x, y))
			}
		}
	}

	cname := C.CString(name)
	//	defer C.free(unsafe.Pointer(cname))

	handle := C.Tk_FindPhoto(interp, cname)
	if handle == nil {
		err := eval("image create photo " + name)
		if err != nil {
			fmt.Println("RRRR")
			return err
		}
		handle = C.Tk_FindPhoto(interp, cname)
		if handle == nil {
			return errors.New("failed to create an image handle")
		}
	}

	imgdata := C.CBytes(nrgba.Pix)
	//	defer C.free(imgdata)

	block := C.Tk_PhotoImageBlock{
		(*C.uchar)(imgdata),
		C.int(nrgba.Rect.Max.X),
		C.int(nrgba.Rect.Max.Y),
		C.int(nrgba.Stride),
		4,
		[...]C.int{0, 1, 2, 3},
	}

	status := C.Tk_PhotoPutBlock(interp, handle, &block, 0, 0,
		C.int(nrgba.Rect.Max.X), C.int(nrgba.Rect.Max.Y),
		C.TK_PHOTO_COMPOSITE_SET)
	if status != C.TCL_OK {
		return errors.New(C.GoString(C.Tcl_GetStringResult(interp)))
	}
	return nil
}
