/*
Copyright 2016 The Kubernetes Authors.

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

package clusterinfo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/Angus-F/cli-runtime/pkg/genericclioptions"
	"github.com/Angus-F/cli-runtime/pkg/printers"
	appsv1client "github.com/Angus-F/client-go/kubernetes/typed/apps/v1"
	corev1client "github.com/Angus-F/client-go/kubernetes/typed/core/v1"
	cmdutil "github.com/Angus-F/kubectl/pkg/cmd/util"
	"github.com/Angus-F/kubectl/pkg/polymorphichelpers"
	"github.com/Angus-F/kubectl/pkg/scheme"
	"github.com/Angus-F/kubectl/pkg/util/i18n"
	"github.com/Angus-F/kubectl/pkg/util/templates"
)

const (
	defaultPodLogsTimeout = 20 * time.Second
	timeout               = 5 * time.Minute
)

type ClusterInfoDumpOptions struct {
	PrintFlags *genericclioptions.PrintFlags
	PrintObj   printers.ResourcePrinterFunc

	OutputDir     string
	AllNamespaces bool
	Namespaces    []string

	Timeout          time.Duration
	AppsClient       appsv1client.AppsV1Interface
	CoreClient       corev1client.CoreV1Interface
	Namespace        string
	RESTClientGetter genericclioptions.RESTClientGetter
	LogsForObject    polymorphichelpers.LogsForObjectFunc

	genericclioptions.IOStreams
}

func NewCmdClusterInfoDump(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &ClusterInfoDumpOptions{
		PrintFlags: genericclioptions.NewPrintFlags("").WithTypeSetter(scheme.Scheme).WithDefaultOutput("json"),

		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:     "dump",
		Short:   i18n.T("Dump lots of relevant info for debugging and diagnosis"),
		Long:    dumpLong,
		Example: dumpExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd))
			cmdutil.CheckErr(o.Run())
		},
	}

	o.PrintFlags.AddFlags(cmd)

	cmd.Flags().StringVar(&o.OutputDir, "output-directory", o.OutputDir, i18n.T("Where to output the files.  If empty or '-' uses stdout, otherwise creates a directory hierarchy in that directory"))
	cmd.Flags().StringSliceVar(&o.Namespaces, "namespaces", o.Namespaces, "A comma separated list of namespaces to dump.")
	cmd.Flags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", o.AllNamespaces, "If true, dump all namespaces.  If true, --namespaces is ignored.")
	cmdutil.AddPodRunningTimeoutFlag(cmd, defaultPodLogsTimeout)
	return cmd
}

var (
	dumpLong = templates.LongDesc(i18n.T(`
    Dumps cluster info out suitable for debugging and diagnosing cluster problems.  By default, dumps everything to
    stdout. You can optionally specify a directory with --output-directory.  If you specify a directory, kubernetes will
    build a set of files in that directory.  By default only dumps things in the current namespace and 'kube-system' namespace, but you can
    switch to a different namespace with the --namespaces flag, or specify --all-namespaces to dump all namespaces.

    The command also dumps the logs of all of the pods in the cluster, these logs are dumped into different directories
    based on namespace and pod name.`))

	dumpExample = templates.Examples(i18n.T(`
    # Dump current cluster state to stdout
    kubectl cluster-info dump

    # Dump current cluster state to /path/to/cluster-state
    kubectl cluster-info dump --output-directory=/path/to/cluster-state

    # Dump all namespaces to stdout
    kubectl cluster-info dump --all-namespaces

    # Dump a set of namespaces to /path/to/cluster-state
    kubectl cluster-info dump --namespaces default,kube-system --output-directory=/path/to/cluster-state`))
)

func setupOutputWriter(dir string, defaultWriter io.Writer, filename string, fileExtension string) io.Writer {
	if len(dir) == 0 || dir == "-" {
		return defaultWriter
	}
	fullFile := path.Join(dir, filename) + fileExtension
	parent := path.Dir(fullFile)
	cmdutil.CheckErr(os.MkdirAll(parent, 0755))

	file, err := os.Create(fullFile)
	cmdutil.CheckErr(err)
	return file
}

func (o *ClusterInfoDumpOptions) Complete(f cmdutil.Factory, cmd *cobra.Command) error {
	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}

	o.PrintObj = printer.PrintObj

	config, err := f.ToRESTConfig()
	if err != nil {
		return err
	}

	o.CoreClient, err = corev1client.NewForConfig(config)
	if err != nil {
		return err
	}

	o.AppsClient, err = appsv1client.NewForConfig(config)
	if err != nil {
		return err
	}

	o.Timeout, err = cmdutil.GetPodRunningTimeoutFlag(cmd)
	if err != nil {
		return err
	}

	o.Namespace, _, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	// TODO this should eventually just be the completed kubeconfigflag struct
	o.RESTClientGetter = f
	o.LogsForObject = polymorphichelpers.LogsForObjectFn

	return nil
}

func (o *ClusterInfoDumpOptions) Run() error {
	nodes, err := o.CoreClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	fileExtension := ".txt"
	if o.PrintFlags.OutputFormat != nil {
		switch *o.PrintFlags.OutputFormat {
		case "json":
			fileExtension = ".json"
		case "yaml":
			fileExtension = ".yaml"
		}
	}

	if err := o.PrintObj(nodes, setupOutputWriter(o.OutputDir, o.Out, "nodes", fileExtension)); err != nil {
		return err
	}

	var namespaces []string
	if o.AllNamespaces {
		namespaceList, err := o.CoreClient.Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		for ix := range namespaceList.Items {
			namespaces = append(namespaces, namespaceList.Items[ix].Name)
		}
	} else {
		if len(o.Namespaces) == 0 {
			namespaces = []string{
				metav1.NamespaceSystem,
				o.Namespace,
			}
		} else {
			namespaces = o.Namespaces
		}
	}
	for _, namespace := range namespaces {
		// TODO: this is repetitive in the extreme.  Use reflection or
		// something to make this a for loop.
		events, err := o.CoreClient.Events(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(events, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "events"), fileExtension)); err != nil {
			return err
		}

		rcs, err := o.CoreClient.ReplicationControllers(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(rcs, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "replication-controllers"), fileExtension)); err != nil {
			return err
		}

		svcs, err := o.CoreClient.Services(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(svcs, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "services"), fileExtension)); err != nil {
			return err
		}

		sets, err := o.AppsClient.DaemonSets(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(sets, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "daemonsets"), fileExtension)); err != nil {
			return err
		}

		deps, err := o.AppsClient.Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(deps, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "deployments"), fileExtension)); err != nil {
			return err
		}

		rps, err := o.AppsClient.ReplicaSets(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if err := o.PrintObj(rps, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "replicasets"), fileExtension)); err != nil {
			return err
		}

		pods, err := o.CoreClient.Pods(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		if err := o.PrintObj(pods, setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, "pods"), fileExtension)); err != nil {
			return err
		}

		printContainer := func(writer io.Writer, container corev1.Container, pod *corev1.Pod) {
			writer.Write([]byte(fmt.Sprintf("==== START logs for container %s of pod %s/%s ====\n", container.Name, pod.Namespace, pod.Name)))
			defer writer.Write([]byte(fmt.Sprintf("==== END logs for container %s of pod %s/%s ====\n", container.Name, pod.Namespace, pod.Name)))

			requests, err := o.LogsForObject(o.RESTClientGetter, pod, &corev1.PodLogOptions{Container: container.Name}, timeout, false)
			if err != nil {
				// Print error and return.
				writer.Write([]byte(fmt.Sprintf("Create log request error: %s\n", err.Error())))
				return
			}

			for _, request := range requests {
				data, err := request.DoRaw(context.TODO())
				if err != nil {
					// Print error and return.
					writer.Write([]byte(fmt.Sprintf("Request log error: %s\n", err.Error())))
					return
				}
				writer.Write(data)
			}
		}

		for ix := range pods.Items {
			pod := &pods.Items[ix]
			initcontainers := pod.Spec.InitContainers
			containers := pod.Spec.Containers
			writer := setupOutputWriter(o.OutputDir, o.Out, path.Join(namespace, pod.Name, "logs"), ".txt")

			for i := range initcontainers {
				printContainer(writer, initcontainers[i], pod)
			}
			for i := range containers {
				printContainer(writer, containers[i], pod)
			}
		}
	}

	dest := o.OutputDir
	if len(dest) > 0 && dest != "-" {
		fmt.Fprintf(o.Out, "Cluster info dumped to %s\n", dest)
	}
	return nil
}
