// SPDX-License-Identifier: GPL-3.0-or-later
package main

/*
#include <stdint.h>
#include <stdlib.h>
char *ds_request(uintptr_t env, uintptr_t activity, const char *action, const char *payload);
void ds_launch(uintptr_t env, uintptr_t activity, const char *action);
*/
import "C"
import (
	"encoding/json"
	"fyne.io/fyne/v2/driver"
	"unsafe"
)

type platformBackend struct{}

func (platformBackend) Request(action string, payload any, result any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	a, p := C.CString(action), C.CString(string(data))
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(p))
	var response string
	err = driver.RunNative(func(ctx any) error {
		c := ctx.(*driver.AndroidContext)
		str := C.ds_request(C.uintptr_t(c.Env), C.uintptr_t(c.Ctx), a, p)
		if str != nil {
			response = C.GoString(str)
			C.free(unsafe.Pointer(str))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return decodeResponse(response, result)
}
func (platformBackend) Launch(action string) error {
	a := C.CString(action)
	defer C.free(unsafe.Pointer(a))
	return driver.RunNative(func(ctx any) error {
		c := ctx.(*driver.AndroidContext)
		C.ds_launch(C.uintptr_t(c.Env), C.uintptr_t(c.Ctx), a)
		return nil
	})
}

//export ds_ui_changed
func ds_ui_changed() { signalChange() }
