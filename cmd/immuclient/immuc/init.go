/*
Copyright 2019-2020 vChain, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package immuc

import (
	c "github.com/codenotary/immudb/cmd/helper"
	"github.com/codenotary/immudb/pkg/client"
	"github.com/spf13/viper"
)

type immuc struct {
	ImmuClient     client.ImmuClient
	passwordReader c.PasswordReader
	valueOnly      bool
	options        *client.Options
	isLoggedin     bool
}

// Client ...
type Client interface {
	Connect(args []string) error
	Disconnect(args []string) error
	CurrentRoot(args []string) (string, error)
	GetByIndex(args []string) (string, error)
	GetKey(args []string) (string, error)
	RawSafeGetKey(args []string) (string, error)
	SafeGetKey(args []string) (string, error)
	GetRawBySafeIndex(args []string) (string, error)
	Login(args []string) (string, error)
	Logout(args []string) (string, error)
	History(args []string) (string, error)
	HealthCheck(args []string) (string, error)
	Reference(args []string) (string, error)
	SafeReference(args []string) (string, error)
	ZScan(args []string) (string, error)
	IScan(args []string) (string, error)
	Scan(args []string) (string, error)
	Count(args []string) (string, error)
	RawSafeSet(args []string) (string, error)
	Set(args []string) (string, error)
	SafeSet(args []string) (string, error)
	ZAdd(args []string) (string, error)
	SafeZAdd(args []string) (string, error)
	Consistency(args []string) (string, error)
	Inclusion(args []string) (string, error)
	ValueOnly() bool
	SetValueOnly(v bool)
	CreateDatabase(args []string) (string, error)
	UseDatabase(args []string) (string, error)
	UserOperations(args []string) (string, error)
}

// Init ...
func Init() (Client, error) {
	ic := new(immuc)
	ic.passwordReader = c.DefaultPasswordReader
	return ic, nil
}

func (i *immuc) Connect(args []string) error {
	i.options = options()
	len, err := client.ReadFileFromUserHomeDir(i.options.TokenFileName)
	if err != nil || len == "" {
		i.options.Auth = false
	} else {
		i.options.Auth = true
	}
	if i.ImmuClient, err = client.NewImmuClient(i.options); err != nil || i.ImmuClient == nil {
		return err
	}
	i.valueOnly = viper.GetBool("value-only")
	return nil
}

func (i *immuc) Disconnect(args []string) error {
	if err := i.ImmuClient.Disconnect(); err != nil {
		return err
	}
	return nil
}

func options() *client.Options {
	options := client.DefaultOptions().
		WithPort(viper.GetInt("immudb-port")).
		WithAddress(viper.GetString("immudb-address")).
		WithTokenFileName(viper.GetString("tokenfile")).
		WithMTLs(viper.GetBool("mtls"))
	if viper.GetBool("mtls") {
		// todo https://golang.org/src/crypto/x509/root_linux.go
		options.MTLsOptions = client.DefaultMTLsOptions().
			WithServername(viper.GetString("servername")).
			WithCertificate(viper.GetString("certificate")).
			WithPkey(viper.GetString("pkey")).
			WithClientCAs(viper.GetString("clientcas"))
	}
	return options
}

func (i *immuc) ValueOnly() bool {
	return i.isLoggedin
}

func (i *immuc) SetValueOnly(v bool) {
	i.isLoggedin = v
	return
}
