package ui

import (
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type SetupConfig struct {
	MySQLVersion    string
	MasterPort      int
	SlavePort       int
	RootPassword    string
	ReplUser        string
	ReplPassword    string
	DatabaseName    string
	NetworkName     string
	BaseDir         string
	MasterContainer string
	SlaveContainer  string
}

type LoadTestFormConfig struct {
	Workers      int
	RequestsPerW int
	PayloadSize  int
	Cleanup      bool
}

type UI struct {
	app            *tview.Application
	pages          *tview.Pages
	header         *tview.TextView
	logView        *tview.TextView
	footer         *tview.TextView
	root           *tview.Flex
	setupHandler   func()
	loadHandler    func()
	statusHander   func()
	downHandler    func()
	resetHandler   func()
	cleanupHandler func()
	currentCfg     *SetupConfig
	loggerSink     interface {
		SetWriter(func(string))
	}
}

func NewUI(loggerSink interface {
	SetWriter(func(string))
}) *UI {
	app := tview.NewApplication()

	header := tview.NewTextView().
		SetDynamicColors(true).
		SetText(HeaderText).
		SetWrap(false)
	header.SetBorder(true).SetTitle(" Overview ")

	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	logView.SetBorder(true).SetTitle(" Activity ")

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow](s)[white] setup  [yellow](l)[white] load test  [yellow](t)[white] status  [yellow](d)[white] down  [yellow](r)[white] reset  [yellow](c)[white] cleanup  [yellow](q)[white] quit")
	footer.SetBorder(true).SetTitle(" Commands ")

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 8, 1, false).
		AddItem(logView, 0, 1, true).
		AddItem(footer, 3, 1, false)

	pages := tview.NewPages().
		AddPage("main", root, true, true)

	u := &UI{
		app:        app,
		pages:      pages,
		header:     header,
		logView:    logView,
		footer:     footer,
		root:       root,
		loggerSink: loggerSink,
	}

	loggerSink.SetWriter(func(msg string) {
		fmt.Fprint(u.logView, msg)
	})

	u.bindKeys()

	return u
}

func (u *UI) bindKeys() {
	u.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			u.app.Stop()
			return nil
		case 's':
			if u.setupHandler != nil {
				u.setupHandler()
			}
			return nil
		case 'l':
			if u.loadHandler != nil {
				u.loadHandler()
			}
			return nil
		case 't':
			if u.statusHander != nil {
				u.statusHander()
			}
			return nil
		case 'd':
			if u.downHandler != nil {
				u.downHandler()
			}
			return nil
		case 'r':
			if u.resetHandler != nil {
				u.resetHandler()
			}
			return nil
		case 'c':
			if u.cleanupHandler != nil {
				u.cleanupHandler()
			}
			return nil
		}
		return event
	})
}

func (u *UI) BindActions(setup func(), load func(), status func(), down func(), reset func(), cleanup func()) {
	u.setupHandler = setup
	u.loadHandler = load
	u.statusHander = status
	u.downHandler = down
	u.resetHandler = reset
	u.cleanupHandler = cleanup
}

func (u *UI) Run() error {
	return u.app.SetRoot(u.pages, true).Run()
}

func (u *UI) ShowConfirmDialog(title, message string) bool {
	doneCh := make(chan bool, 1)

	dialog := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			doneCh <- buttonLabel == "Yes"
			u.pages.RemovePage("confirm")
			u.app.SetFocus(u.root)
		})

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(dialog, 60, 1, true).
		AddItem(nil, 0, 1, false)

	u.app.QueueUpdateDraw(func() {
		u.pages.AddPage("confirm", flex, true, true)
		u.app.SetFocus(dialog)
	})

	return <-doneCh
}

func (u *UI) CurrentConfig() *SetupConfig {
	return u.currentCfg
}

func (u *UI) ShowSetupForm() (*SetupConfig, error) {
	resultCh := make(chan *SetupConfig, 1)

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Interactive Setup ").SetTitleAlign(tview.AlignLeft)

	mysqlVersion := "8.0"
	masterPort := "33060"
	slavePort := "33061"
	rootPassword := "123456@Aa"
	replUser := "slave"
	replPassword := "123456@Aa"
	dbName := "test_db"
	networkName := "myreplica-net"
	baseDir := "./workdir"
	masterContainer := "mysql-master"
	slaveContainer := "mysql-slave"

	form.
		AddInputField("MySQL Version", mysqlVersion, 20, nil, func(text string) { mysqlVersion = text }).
		AddInputField("Master Port", masterPort, 20, nil, func(text string) { masterPort = text }).
		AddInputField("Slave Port", slavePort, 20, nil, func(text string) { slavePort = text }).
		AddPasswordField("Root Password", rootPassword, 30, '*', func(text string) { rootPassword = text }).
		AddInputField("Replication User", replUser, 20, nil, func(text string) { replUser = text }).
		AddPasswordField("Replication Password", replPassword, 30, '*', func(text string) { replPassword = text }).
		AddInputField("Database Name", dbName, 30, nil, func(text string) { dbName = text }).
		AddInputField("Docker Network", networkName, 30, nil, func(text string) { networkName = text }).
		AddInputField("Workdir", baseDir, 50, nil, func(text string) { baseDir = text }).
		AddInputField("Master Container", masterContainer, 30, nil, func(text string) { masterContainer = text }).
		AddInputField("Slave Container", slaveContainer, 30, nil, func(text string) { slaveContainer = text }).
		AddButton("Start", func() {
			mp, _ := strconv.Atoi(masterPort)
			sp, _ := strconv.Atoi(slavePort)

			cfg := &SetupConfig{
				MySQLVersion:    mysqlVersion,
				MasterPort:      mp,
				SlavePort:       sp,
				RootPassword:    rootPassword,
				ReplUser:        replUser,
				ReplPassword:    replPassword,
				DatabaseName:    dbName,
				NetworkName:     networkName,
				BaseDir:         baseDir,
				MasterContainer: masterContainer,
				SlaveContainer:  slaveContainer,
			}

			u.currentCfg = cfg
			resultCh <- cfg
			u.pages.RemovePage("setup")
			u.app.SetFocus(u.root)
		}).
		AddButton("Cancel", func() {
			close(resultCh)
			u.pages.RemovePage("setup")
			u.app.SetFocus(u.root)
		})

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(form, 80, 1, true).
		AddItem(nil, 0, 1, false)

	u.app.QueueUpdateDraw(func() {
		u.pages.AddPage("setup", modal, true, true)
		u.app.SetFocus(form)
	})

	cfg, ok := <-resultCh
	if !ok {
		return nil, fmt.Errorf("cancelled")
	}

	return cfg, nil
}

func (u *UI) ShowLoadTestForm() (LoadTestFormConfig, bool) {
	resultCh := make(chan LoadTestFormConfig, 1)
	cancelCh := make(chan struct{}, 1)

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Load Test Settings ").SetTitleAlign(tview.AlignLeft)

	workers := "4"
	requestsPerW := "250"
	payloadSize := "128"
	cleanup := "false"

	form.
		AddInputField("Workers", workers, 10, nil, func(text string) { workers = text }).
		AddInputField("Requests/Worker", requestsPerW, 10, nil, func(text string) { requestsPerW = text }).
		AddInputField("Payload Size (bytes)", payloadSize, 10, nil, func(text string) { payloadSize = text }).
		AddCheckbox("Cleanup table after test", false, func(checked bool) {
			if checked {
				cleanup = "true"
			} else {
				cleanup = "false"
			}
		}).
		AddButton("Run", func() {
			w, _ := strconv.Atoi(workers)
			r, _ := strconv.Atoi(requestsPerW)
			p, _ := strconv.Atoi(payloadSize)

			resultCh <- LoadTestFormConfig{
				Workers:      w,
				RequestsPerW: r,
				PayloadSize:  p,
				Cleanup:      cleanup == "true",
			}
			u.pages.RemovePage("loadtest")
			u.app.SetFocus(u.root)
		}).
		AddButton("Cancel", func() {
			cancelCh <- struct{}{}
			u.pages.RemovePage("loadtest")
			u.app.SetFocus(u.root)
		})

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(form, 70, 1, true).
		AddItem(nil, 0, 1, false)

	u.app.QueueUpdateDraw(func() {
		u.pages.AddPage("loadtest", modal, true, true)
		u.app.SetFocus(form)
	})

	select {
	case cfg := <-resultCh:
		return cfg, true
	case <-cancelCh:
		return LoadTestFormConfig{}, false
	}
}
