/*
Copyright 2019 The Kubernetes Authors.

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

package rollout

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"github.com/Angus-F/cli-runtime/pkg/genericclioptions"
	"github.com/Angus-F/cli-runtime/pkg/printers"
	"github.com/Angus-F/cli-runtime/pkg/resource"
	"github.com/Angus-F/kubectl/pkg/cmd/set"
	cmdutil "github.com/Angus-F/kubectl/pkg/cmd/util"
	"github.com/Angus-F/kubectl/pkg/polymorphichelpers"
	"github.com/Angus-F/kubectl/pkg/scheme"
	"github.com/Angus-F/kubectl/pkg/util/i18n"
	"github.com/Angus-F/kubectl/pkg/util/templates"
)

// RestartOptions is the start of the data required to perform the operation.  As new fields are added, add them here instead of
// referencing the cmd.Flags()
type RestartOptions struct {
	PrintFlags *genericclioptions.PrintFlags
	ToPrinter  func(string) (printers.ResourcePrinter, error)

	Resources []string

	Builder          func() *resource.Builder
	Restarter        polymorphichelpers.ObjectRestarterFunc
	Namespace        string
	EnforceNamespace bool

	resource.FilenameOptions
	genericclioptions.IOStreams

	fieldManager string
}

var (
	restartLong = templates.LongDesc(i18n.T(`
		Restart a resource.

	        Resource will be rollout restarted.`))

	restartExample = templates.Examples(`
		# Restart a deployment
		kubectl rollout restart deployment/nginx

		# Restart a daemonset
		kubectl rollout restart daemonset/abc`)
)

// NewRolloutRestartOptions returns an initialized RestartOptions instance
func NewRolloutRestartOptions(streams genericclioptions.IOStreams) *RestartOptions {
	return &RestartOptions{
		PrintFlags: genericclioptions.NewPrintFlags("restarted").WithTypeSetter(scheme.Scheme),
		IOStreams:  streams,
	}
}

// NewCmdRolloutRestart returns a Command instance for 'rollout restart' sub command
func NewCmdRolloutRestart(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRolloutRestartOptions(streams)

	validArgs := []string{"deployment", "daemonset", "statefulset"}

	cmd := &cobra.Command{
		Use:                   "restart RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Restart a resource"),
		Long:                  restartLong,
		Example:               restartExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.RunRestart())
		},
		ValidArgs: validArgs,
	}

	usage := "identifying the resource to get from a server."
	cmdutil.AddFilenameOptionFlags(cmd, &o.FilenameOptions, usage)
	cmdutil.AddFieldManagerFlagVar(cmd, &o.fieldManager, "kubectl-rollout")
	o.PrintFlags.AddFlags(cmd)
	return cmd
}

// Complete completes all the required options
func (o *RestartOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	o.Resources = args

	o.Restarter = polymorphichelpers.ObjectRestarterFn

	var err error
	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.ToPrinter = func(operation string) (printers.ResourcePrinter, error) {
		o.PrintFlags.NamePrintFlags.Operation = operation
		return o.PrintFlags.ToPrinter()
	}

	o.Builder = f.NewBuilder

	return nil
}

func (o *RestartOptions) Validate() error {
	if len(o.Resources) == 0 && cmdutil.IsFilenameSliceEmpty(o.Filenames, o.Kustomize) {
		return fmt.Errorf("required resource not specified")
	}
	return nil
}

// RunRestart performs the execution of 'rollout restart' sub command
func (o RestartOptions) RunRestart() error {
	r := o.Builder().
		WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
		NamespaceParam(o.Namespace).DefaultNamespace().
		FilenameParam(o.EnforceNamespace, &o.FilenameOptions).
		ResourceTypeOrNameArgs(true, o.Resources...).
		ContinueOnError().
		Latest().
		Flatten().
		Do()
	if err := r.Err(); err != nil {
		return err
	}

	allErrs := []error{}
	infos, err := r.Infos()
	if err != nil {
		// restore previous command behavior where
		// an error caused by retrieving infos due to
		// at least a single broken object did not result
		// in an immediate return, but rather an overall
		// aggregation of errors.
		allErrs = append(allErrs, err)
	}

	for _, patch := range set.CalculatePatches(infos, scheme.DefaultJSONEncoder(), set.PatchFn(o.Restarter)) {
		info := patch.Info

		if patch.Err != nil {
			resourceString := info.Mapping.Resource.Resource
			if len(info.Mapping.Resource.Group) > 0 {
				resourceString = resourceString + "." + info.Mapping.Resource.Group
			}
			allErrs = append(allErrs, fmt.Errorf("error: %s %q %v", resourceString, info.Name, patch.Err))
			continue
		}

		if string(patch.Patch) == "{}" || len(patch.Patch) == 0 {
			allErrs = append(allErrs, fmt.Errorf("failed to create patch for %v: empty patch", info.Name))
		}

		obj, err := resource.NewHelper(info.Client, info.Mapping).
			WithFieldManager(o.fieldManager).
			Patch(info.Namespace, info.Name, types.StrategicMergePatchType, patch.Patch, nil)
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("failed to patch: %v", err))
			continue
		}

		info.Refresh(obj, true)
		printer, err := o.ToPrinter("restarted")
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		if err = printer.PrintObj(info.Object, o.Out); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return utilerrors.NewAggregate(allErrs)
}
