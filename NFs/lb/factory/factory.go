/*
 * LB Configuration Factory
 */

package factory

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"

	"loadbalance/logger"
)

var LbConfig Config

// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		return err
	} else {
		LbConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &LbConfig); yamlErr != nil {
			return yamlErr
		}
	}

	return nil
}

func CheckConfigVersion() error {
	currentVersion := LbConfig.GetVersion()

	if currentVersion != LB_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s].",
			currentVersion, LB_EXPECTED_CONFIG_VERSION)
	}

	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}
