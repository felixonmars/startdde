/*
 * Copyright (C) 2016 ~ 2018 Deepin Technology Co., Ltd.
 *
 * Author:     jouyouyun <jouyouwen717@gmail.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package watchdog

import (
	"os"
	"strconv"
	"time"

	"pkg.deepin.io/lib/dbus"
	"pkg.deepin.io/lib/log"
	"pkg.deepin.io/lib/procfs"
)

const (
	envMaxLaunchTimes = "DDE_WATCHDOG_MAX_LAUNCH_TIMES"
)

var (
	logger   = log.NewLogger("daemon/watchdog")
	_manager *Manager
	// if times == 0, unlimit
	maxLaunchTimes = 10
)

func Start() {
	if _manager != nil {
		return
	}

	err := initDBusObject()
	if err != nil {
		logger.Warning(err)
		return
	}

	times := os.Getenv(envMaxLaunchTimes)
	if len(times) != 0 {
		v, err := strconv.ParseInt(times, 10, 64)
		if err == nil {
			maxLaunchTimes = int(v)
		}
	}
	logger.Debug("[WATCHDOG] max launch times:", maxLaunchTimes)
	_manager = newManager()
	_manager.AddTask(newDockTask())
	_manager.AddTask(newDesktopTask())
	_manager.AddTask(newDDEPolkitAgent())
	go _manager.StartLoop()

	wmTask := newWMTask()
	_manager.taskMap = map[string]*taskInfo{
		wmDest: wmTask,
	}

	go func() {
		time.Sleep(10 * time.Second)
		isRun, err := wmTask.isRunning()
		if err != nil {
			logger.Warning(err)
			return
		}

		if !isRun {
			err := wmTask.launcher()
			if err != nil {
				logger.Warning(err)
			}
		}

		_manager.listenNameOwnerChanged()
	}()
	return
}

func (m *Manager) listenNameOwnerChanged() error {
	bus, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	signalChan := bus.Signal()

	rule := "type='signal',interface='org.freedesktop.DBus',member='NameOwnerChanged'"
	err = bus.BusObject().Call(orgFreedesktopDBus+".AddMatch", 0, rule).Err
	if err != nil {
		return err
	}

	rule = "type='signal',interface='com.deepin.WMSwitcher',member='WMChanged'"
	err = bus.BusObject().Call(orgFreedesktopDBus+".AddMatch", 0, rule).Err
	if err != nil {
		return err
	}

	go func() {
		for signal := range signalChan {
			if signal.Name == orgFreedesktopDBus+".NameOwnerChanged" &&
				signal.Path == "/org/freedesktop/DBus" && len(signal.Body) == 3 {
				name, ok := signal.Body[0].(string)
				if !ok {
					continue
				}
				taskInfo := m.taskMap[name]
				if taskInfo == nil {
					continue
				}

				oldOwner, ok := signal.Body[1].(string)
				if !ok {
					continue
				}

				newOwner, ok := signal.Body[2].(string)
				if !ok {
					continue
				}

				if oldOwner != "" && newOwner == "" {
					logger.Debugf("name lost %q, old owner: %q", name, oldOwner)

					go func() {
						logger.Debug("sleep 3s")
						time.Sleep(3 * time.Second)
						err := taskInfo.Launch()
						if err != nil {
							logger.Warningf("failed to launch task %s: %v", taskInfo.Name, err)
						}
					}()

				} else if oldOwner == "" && newOwner != "" {
					logger.Debugf("name acquired %q, new owner: %q", name, newOwner)
					var pid uint32
					err := busObj.Call(orgFreedesktopDBus+".GetConnectionUnixProcessID", 0,
						newOwner).Store(&pid)
					if err != nil {
						logger.Warningf("failed to get conn %q pid: %v", newOwner, err)
						continue
					}

					process := procfs.Process(pid)
					exe, err := process.Exe()
					if err != nil {
						logger.Warning("failed to get process %d exe:", pid)
						continue
					}
					logger.Debugf("exe: %q", exe)
				}
			} else if signal.Name == "com.deepin.WMSwitcher.WMChanged" &&
				signal.Path == "/com/deepin/WMSwitcher" && len(signal.Body) == 1 {
				name, ok := signal.Body[0].(string)
				if !ok {
					continue
				}
				logger.Debugf("wm changed %q", name)
			}
		}

	}()
	return nil
}

func Stop() {
	if _manager == nil {
		return
	}

	_manager.QuitLoop()
	_manager = nil
	return
}

func SetLogLevel(level log.Priority) {
	logger.SetLogLevel(level)
}
