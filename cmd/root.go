// Package cmd is the root command for the eventsrunner-k8s-sensor application.
/*
Copyright © 2021 Luqman Mohammed m.luqman077@gmail.com

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"flag"
	"strconv"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var cfgPath string
var logVerbosity int

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "er-k8s-sensor",
	Short: "K8s sensor to sense events from K8s and process them",
	Long: `K8s sensor to do event driven automation by sensing events 
	according to the configured rules from kubernetes and process them 
	using the configured executor.`,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := config.ParseConfigFromViper(cfgPath, logVerbosity)
		if err != nil {
			panic(err)
		}
		klog.InitFlags(nil)
		flag.Set("v", strconv.Itoa(config.LogVerbosity))
		defer klog.Flush()
		sr, err := sensor.SetupNewSensorRuntime(config)
		if err != nil {
			klog.V(1).ErrorS(err, "Failed to setup sensor runtime")
			panic(err)
		}
		go sr.StartSensorRuntime()
		sr.StopOnSignal()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "", "c", "config file (default is $HOME/.er-k8s-sensor/config.yaml)")
	rootCmd.PersistentFlags().IntVarP(&logVerbosity, "verbosity", "v", 0, "log verbosity")
}
