/*
 * Copyright (C) 2014 ~ 2018 Deepin Technology Co., Ltd.
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

package xsettings

import (
	"fmt"

	"gir/gio-2.0"
	"github.com/linuxdeepin/go-x11-client"
	"pkg.deepin.io/lib/dbus"
	"pkg.deepin.io/lib/gsettings"
	"pkg.deepin.io/lib/log"
)

const (
	xsSchema           = "com.deepin.xsettings"
	defaultScaleFactor = 1.0
)

var logger *log.Logger

// XSManager xsettings manager
type XSManager struct {
	conn  *x.Conn
	owner x.Window

	gs *gio.Settings

	SetScaleFactorDone func()
	restartOSD         bool // whether to restart dde-osd
}

type xsSetting struct {
	sType int8
	prop  string
	value interface{} // int32, string, [4]int16
}

func NewXSManager(conn *x.Conn, recommendedScaleFactor float64) (*XSManager, error) {
	var m = &XSManager{
		conn: conn,
	}

	var err error
	m.owner, err = createSettingWindow(m.conn)
	if err != nil {
		return nil, err
	}
	logger.Debug("owner:", m.owner)

	if !isSelectionOwned(settingPropScreen, m.owner, m.conn) {
		logger.Errorf("Owned '%s' failed", settingPropSettings)
		return nil, fmt.Errorf("Owned '%s' failed", settingPropSettings)
	}
	logger.Debug("recommended scale factor:", recommendedScaleFactor)

	m.gs = gio.NewSettings(xsSchema)
	if m.gs.GetUserValue(gsKeyScaleFactor) == nil &&
		recommendedScaleFactor != defaultScaleFactor {
		m.setScaleFactor(recommendedScaleFactor, false)
		m.restartOSD = true
	}

	err = m.setSettings(m.getSettingsInSchema())
	if err != nil {
		logger.Warning("Change xsettings property failed:", err)
	}

	return m, nil
}

func (m *XSManager) setSettings(settings []xsSetting) error {
	datas, err := getSettingPropValue(m.owner, m.conn)
	if err != nil {
		return err
	}

	xsInfo := marshalSettingData(datas)
	xsInfo.serial = xsDataSerial
	for _, s := range settings {
		item := xsInfo.getPropItem(s.prop)
		if item != nil {
			xsInfo.items = xsInfo.modifyProperty(s)
			continue
		}

		var tmp *xsItemInfo
		switch s.sType {
		case settingTypeInteger:
			tmp = newXSItemInteger(s.prop, s.value.(int32))
		case settingTypeString:
			tmp = newXSItemString(s.prop, s.value.(string))
		case settingTypeColor:
			tmp = newXSItemColor(s.prop, s.value.([4]int16))
		}

		xsInfo.items = append(xsInfo.items, *tmp)
		xsInfo.numSettings++
	}

	data := unmarshalSettingData(xsInfo)
	return changeSettingProp(m.owner, data, m.conn)
}

func (m *XSManager) getSettingsInSchema() []xsSetting {
	var settings []xsSetting
	for _, key := range m.gs.ListKeys() {
		info := gsInfos.getInfoByGSKey(key)
		if info == nil {
			continue
		}

		settings = append(settings, xsSetting{
			sType: info.getKeySType(),
			prop:  info.xsKey,
			value: info.getKeyValue(m.gs),
		})
	}

	return settings
}

func (m *XSManager) handleGSettingsChanged() {
	gsettings.ConnectChanged(xsSchema, "*", func(key string) {
		switch key {
		case "xft-dpi":
			return
		case "scale-factor":
			m.updateDPI()
			return
		case "gtk-cursor-theme-name":
			updateXResources(xresourceInfos{
				&xresourceInfo{
					key:   "Xcursor.theme",
					value: m.gs.GetString("gtk-cursor-theme-name"),
				},
			})
		case "gtk-cursor-theme-size":
			updateXResources(xresourceInfos{
				&xresourceInfo{
					key:   "Xcursor.size",
					value: fmt.Sprintf("%d", m.gs.GetInt("gtk-cursor-theme-size")),
				},
			})
		case "window-scale":
			m.updateDPI()
			return
		}

		info := gsInfos.getInfoByGSKey(key)
		if info == nil {
			return
		}

		m.setSettings([]xsSetting{{
			sType: info.getKeySType(),
			prop:  info.xsKey,
			value: info.getKeyValue(m.gs),
		},
		})
	})
}

// Start load xsettings module
func Start(conn *x.Conn, l *log.Logger, recommendedScaleFactor float64) (*XSManager, error) {
	logger = l
	m, err := NewXSManager(conn, recommendedScaleFactor)
	if err != nil {
		logger.Error("Start xsettings failed:", err)
		return nil, err
	}
	m.updateDPI()
	m.updateXResources()
	go m.updateFirefoxDPI()

	err = dbus.InstallOnSession(m)
	if err != nil {
		logger.Error("Install dbus session failed:", err)
		return nil, err
	}
	dbus.DealWithUnhandledMessage()

	m.handleGSettingsChanged()
	return m, nil
}

func (m *XSManager) NeedRestartOSD() bool {
	if m == nil {
		return false
	}
	return m.restartOSD
}
