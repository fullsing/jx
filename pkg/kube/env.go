package kube

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx/pkg/util"
	"gopkg.in/AlecAivazis/survey.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateEnvironmentSurvey creates a Survey on the given environment using the default options
// from the CLI
func CreateEnvironmentSurvey(data *v1.Environment, config *v1.Environment, noGitOps bool, ns string, jxClient *versioned.Clientset) error {
	name := data.Name
	createMode := name == ""
	if createMode {
		if config.Name != "" {
			err := ValidNameOption(OptionName, config.Name)
			if err != nil {
				return err
			}
			err = ValidateEnvironmentDoesNotExist(jxClient, ns, config.Name)
			if err != nil {
				return util.InvalidOptionError(OptionName, config.Name, err)
			}
			data.Name = config.Name
		} else {
			validator := func(val interface{}) error {
				err := ValidateName(val)
				if err != nil {
					return err
				}
				str, ok := val.(string)
				if !ok {
					return fmt.Errorf("Expected string value!")
				}
				return ValidateEnvironmentDoesNotExist(jxClient, ns, str)
			}

			q := &survey.Input{
				Message: "Name:",
				Help:    "The Environment name must be unique, lower case and a valid DNS name",
			}
			err := survey.AskOne(q, &data.Name, validator)
			if err != nil {
				return err
			}
		}
	}
	if config.Spec.Label != "" {
		data.Spec.Label = config.Spec.Label
	} else {
		defaultValue := data.Spec.Label
		if defaultValue == "" {
			defaultValue = strings.Title(data.Name)
		}
		q := &survey.Input{
			Message: "Label:",
			Default: defaultValue,
			Help:    "The Environment label is a person friendly descriptive text like 'Staging' or 'Production'",
		}
		err := survey.AskOne(q, &data.Spec.Label, survey.Required)
		if err != nil {
			return err
		}
	}
	if config.Spec.Namespace != "" {
		err := ValidNameOption(OptionNamespace, config.Spec.Namespace)
		if err != nil {
			return err
		}
		data.Spec.Namespace = config.Spec.Namespace
	} else {
		defaultValue := data.Spec.Namespace
		if defaultValue == "" {
			// lets use the namespace as a team name
			defaultValue = data.Namespace
			if defaultValue == "" {
				defaultValue = ns
			}
			if data.Name != "" {
				if defaultValue == "" {
					defaultValue = data.Name
				} else {
					defaultValue += "-" + data.Name
				}
			}
		}
		q := &survey.Input{
			Message: "Namespace:",
			Default: defaultValue,
			Help:    "Th kubernetes namespace name to use for this Environment",
		}
		err := survey.AskOne(q, &data.Spec.Namespace, ValidateName)
		if err != nil {
			return err
		}
	}
	if config.Spec.Cluster != "" {
		data.Spec.Cluster = config.Spec.Cluster
	} else {
		// lets not show the UI for this if users specify the namespace via arguments
		if !createMode || config.Spec.Namespace == "" {
			defaultValue := data.Spec.Cluster
			q := &survey.Input{
				Message: "Cluster URL:",
				Default: defaultValue,
				Help:    "The kubernetes cluster URL to use to host this Environment",
			}
			// TODO validate/transform to match valid kubnernetes cluster syntax
			err := survey.AskOne(q, &data.Spec.Cluster, nil)
			if err != nil {
				return err
			}
		}
	}
	if string(config.Spec.PromotionStrategy) != "" {
		data.Spec.PromotionStrategy = config.Spec.PromotionStrategy
	} else {
		promoteValues := []string{
			string(v1.PromotionStrategyTypeAutomatic),
			string(v1.PromotionStrategyTypeManual),
			string(v1.PromotionStrategyTypeNever),
		}
		defaultValue := string(data.Spec.PromotionStrategy)
		if defaultValue == "" {
			defaultValue = string(v1.PromotionStrategyTypeAutomatic)
		}
		q := &survey.Select{
			Message: "Promotion Strategy:",
			Options: promoteValues,
			Default: defaultValue,
			Help:    "Whether we promote to this Environment automatically, manually or never",
		}
		textValue := ""
		err := survey.AskOne(q, &textValue, survey.Required)
		if err != nil {
			return err
		}
		if textValue != "" {
			data.Spec.PromotionStrategy = v1.PromotionStrategyType(textValue)
		}
	}
	if config.Spec.Order != 0 {
		data.Spec.Order = config.Spec.Order
	} else {
		order := data.Spec.Order
		if order == 0 {
			// TODO should we generate an order to default to last one?
			order = 100
		}
		defaultValue := util.Int32ToA(order)
		q := &survey.Input{
			Message: "Order:",
			Default: defaultValue,
			Help:    "This number is used to sort Environments in sequential order, lowest first",
		}
		textValue := ""
		err := survey.AskOne(q, &textValue, survey.Required)
		if err != nil {
			return err
		}
		if textValue != "" {
			i, err := util.AtoInt32(textValue)
			if err != nil {
				return fmt.Errorf("Failed to convert input '%s' to number: %s", textValue, err)
			}
			data.Spec.Order = i
		}
	}
	if string(data.Spec.PromotionStrategy) == "" {
		data.Spec.PromotionStrategy = v1.PromotionStrategyTypeAutomatic
	}
	return nil
}

// GetEnvironmentNames returns the sorted list of environment names
func GetEnvironmentNames(jxClient *versioned.Clientset, ns string) ([]string, error) {
	envNames := []string{}
	envs, err := jxClient.JenkinsV1().Environments(ns).List(metav1.ListOptions{})
	if err != nil {
		return envNames, err
	}
	for _, env := range envs.Items {
		n := env.Name
		if n != "" {
			envNames = append(envNames, n)
		}
	}
	sort.Strings(envNames)
	return envNames, nil
}

// GetEnvironments returns a map of the enviroments along with a sorted list of names
func GetEnvironments(jxClient *versioned.Clientset, ns string) (map[string]*v1.Environment, []string, error) {
	m := map[string]*v1.Environment{}

	envNames := []string{}
	envs, err := jxClient.JenkinsV1().Environments(ns).List(metav1.ListOptions{})
	if err != nil {
		return m, envNames, err
	}
	for _, env := range envs.Items {
		n := env.Name
		m[n] = &env
		if n != "" {
			envNames = append(envNames, n)
		}
	}
	sort.Strings(envNames)
	return m, envNames, nil
}

// GetDevNamespace returns the developer environment namespace
// which is the namespace that contains the Environments and the developer tools like Jenkins
func GetDevNamespace(kubeClient *kubernetes.Clientset, ns string) (string, string, error) {
	env := ""
	namespace, err := kubeClient.CoreV1().Namespaces().Get(ns, metav1.GetOptions{})
	if err != err {
		return ns, env, err
	}
	if namespace.Labels != nil {
		answer := namespace.Labels[LabelTeam]
		if answer != "" {
			ns = answer
		}
		env = namespace.Labels[LabelEnvironment]
	}
	return ns, env, nil
}

func PickEnvironment(envNames []string) (string, error) {
	name := ""
	if len(envNames) == 0 {
		return "", nil
	} else if len(envNames) == 1 {
		name = envNames[0]
	} else {
		prompt := &survey.Select{
			Message: "Pick environment:",
			Options: envNames,
		}
		err := survey.AskOne(prompt, &name, nil)
		if err != nil {
			return "", err
		}
	}
	return name, nil
}

func PickName(names []string, message string) (string, error) {
	name := ""
	if len(names) == 0 {
		return "", nil
	} else if len(names) == 1 {
		name = names[0]
	} else {
		prompt := &survey.Select{
			Message: message,
			Options: names,
		}
		err := survey.AskOne(prompt, &name, nil)
		if err != nil {
			return "", err
		}
	}
	return name, nil
}
