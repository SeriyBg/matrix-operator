/*
Copyright 2022.

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

package v1alpha1

import (
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var oraclelog = logf.Log.WithName("oracle-resource")

func (r *Oracle) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-matrix-operator-com-v1alpha1-oracle,mutating=true,failurePolicy=fail,sideEffects=None,groups=matrix.operator.com,resources=oracles,verbs=create;update,versions=v1alpha1,name=moracle.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Oracle{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Oracle) Default() {
	oraclelog.Info("default", "name", r.Name)
	if r.Spec.Config == nil {
		r.Spec.Config.Names = []string{"Apoc", "Choi", "DuJour", "Cypher", "Dozer", "Morpheus", "Mouse", "Neo"}
		r.Spec.Config.Predictions = []string{
			"Know Thyself.",
			"You're cuter than I thought. I can see why she likes you.",
			"Would You Still Have Broken It If I Hadn't Said Anything?",
		}
	}
}

//+kubebuilder:webhook:path=/validate-matrix-operator-com-v1alpha1-oracle,mutating=false,failurePolicy=fail,sideEffects=None,groups=matrix.operator.com,resources=oracles,verbs=create;update,versions=v1alpha1,name=voracle.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Oracle{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Oracle) ValidateCreate() error {
	oraclelog.Info("validate create", "name", r.Name)

	return r.validateOracleNames()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Oracle) ValidateUpdate(old runtime.Object) error {
	oraclelog.Info("validate update", "name", r.Name)

	return r.validateOracleNames()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Oracle) ValidateDelete() error {
	oraclelog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *Oracle) validateOracleNames() error {
	if r.Spec.Config == nil {
		return nil
	}
	var allErrs field.ErrorList
	compile := regexp.MustCompile(`\d`)
	for _, name := range r.Spec.Config.Names {
		if compile.MatchString(name) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("config").Child("names"), name, "numbers are not allowed in names"))
		}
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: "matrix.operator.com", Kind: "Oracle"}, r.Name, allErrs)
}
