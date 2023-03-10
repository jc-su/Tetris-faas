// Copyright (c) Alex Ellis 2017. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	types "github.com/openfaas/faas-provider/types"
	"github.com/openfaas/faas/gateway/handlers"
	"github.com/openfaas/faas/gateway/scaling"
)

// NewExternalServiceQuery proxies service queries to external plugin via HTTP
func NewExternalServiceQuery(externalURL url.URL, authInjector handlers.AuthInjector) scaling.ServiceQuery {
	timeout := 3 * time.Second

	proxyClient := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 0,
			}).DialContext,
			MaxIdleConns:          1,
			DisableKeepAlives:     true,
			IdleConnTimeout:       120 * time.Millisecond,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
	}

	return ExternalServiceQuery{
		URL:          externalURL,
		ProxyClient:  proxyClient,
		AuthInjector: authInjector,
	}
}

// ExternalServiceQuery proxies service queries to external plugin via HTTP
type ExternalServiceQuery struct {
	URL          url.URL
	ProxyClient  http.Client
	AuthInjector handlers.AuthInjector
}

// ScaleServiceRequest request scaling of replica
type ScaleServiceRequest struct {
	ServiceName      string `json:"serviceName"`
	ServiceNamespace string `json:"serviceNamespace"`
	Replicas         uint64 `json:"replicas"`
}

// GetReplicas replica count for function
func (s ExternalServiceQuery) GetReplicas(serviceName, serviceNamespace string) (scaling.ServiceQueryResponse, error) {
	start := time.Now()

	var err error
	var emptyServiceQueryResponse scaling.ServiceQueryResponse

	function := types.FunctionStatus{}

	urlPath := fmt.Sprintf("%ssystem/function/%s?namespace=%s", s.URL.String(), serviceName, serviceNamespace)

	req, _ := http.NewRequest(http.MethodGet, urlPath, nil)

	if s.AuthInjector != nil {
		s.AuthInjector.Inject(req)
	}

	res, err := s.ProxyClient.Do(req)

	if err != nil {
		log.Println(urlPath, err)
	} else {

		if res.Body != nil {
			defer res.Body.Close()
		}

		if res.StatusCode == http.StatusOK {
			bytesOut, _ := ioutil.ReadAll(res.Body)
			err = json.Unmarshal(bytesOut, &function)
			if err != nil {
				log.Println(urlPath, err)
			}
			log.Printf("GetReplicas [%s.%s] took: %fs, replicas=%d/%d\n",
				serviceName,
				serviceNamespace,
				time.Since(start).Seconds(),
				function.AvailableReplicas,
				function.Replicas)

		} else {
			//log.Printf("GetReplicas [%s.%s] took: %fs, code: %d \n", serviceName, serviceNamespace, time.Since(start).Seconds(), res.StatusCode)
			return emptyServiceQueryResponse, fmt.Errorf("server returned non-200 status code (%d) for function, %s \n", res.StatusCode, serviceName)
		}
	}

	/*minReplicas := uint64(scaling.DefaultMinReplicas)
	maxReplicas := uint64(scaling.DefaultMaxReplicas)
	scalingFactor := uint64(scaling.DefaultScalingFactor)


	if function.Labels != nil {
		labels := *function.Labels

		minReplicas = extractLabelValue(labels[scaling.MinScaleLabel], minReplicas)
		maxReplicas = extractLabelValue(labels[scaling.MaxScaleLabel], maxReplicas)
		extractedScalingFactor := extractLabelValue(labels[scaling.ScalingFactorLabel], scalingFactor)

		if extractedScalingFactor >= 0 && extractedScalingFactor <= 100 {
			scalingFactor = extractedScalingFactor
		} else {
			log.Printf("Bad Scaling Factor: %d, is not in range of [0 - 100]. Will fallback to %d \n", extractedScalingFactor, scalingFactor)
		}
	}*/
	//log.Printf("GetReplicas [%s.%s] took: %fs", serviceName, serviceNamespace, time.Since(start).Seconds())

	return scaling.ServiceQueryResponse{
		Replicas:          function.Replicas,
		//MaxReplicas:       maxReplicas,
		//MinReplicas:       minReplicas,
		//ScalingFactor:     scalingFactor,
		AvailableReplicas: function.AvailableReplicas,
	}, err
}

// SetReplicas update the replica count
func (s ExternalServiceQuery) SetReplicas(serviceName, serviceNamespace string, count uint64) error {
	start := time.Now()

	var err error
	scaleReq := ScaleServiceRequest{
		ServiceName:      serviceName,
		Replicas:         count,
		ServiceNamespace: serviceNamespace,
	}

	requestBody, err := json.Marshal(scaleReq)
	if err != nil {
		return err
	}

	urlPath := fmt.Sprintf("%ssystem/scale-function/%s?namespace=%s", s.URL.String(), serviceName, serviceNamespace)
	req, _ := http.NewRequest(http.MethodPost, urlPath, bytes.NewReader(requestBody))

	if s.AuthInjector != nil {
		s.AuthInjector.Inject(req)
	}

	defer req.Body.Close()
	res, err := s.ProxyClient.Do(req)

	if err != nil {
		log.Println(urlPath, err)
	} else {
		if res.Body != nil {
			defer res.Body.Close()
		}
	}

	if !(res.StatusCode == http.StatusOK || res.StatusCode == http.StatusAccepted) {
		log.Printf("SetReplicas [%s.%s] took: %fs, code: %d \n", serviceName, serviceNamespace, time.Since(start).Seconds(), res.StatusCode)
		err = fmt.Errorf("error scaling HTTP code %d, %s \n", res.StatusCode, urlPath)
		return err
	} else {
		log.Printf("SetReplicas [%s.%s] took: %fs", serviceName, serviceNamespace, time.Since(start).Seconds())
		return nil
	}
}

// extractLabelValue will parse the provided raw label value and if it fails
// it will return the provided fallback value and log an message
/*func extractLabelValue(rawLabelValue string, fallback uint64) uint64 {
	if len(rawLabelValue) <= 0 {
		return fallback
	}

	value, err := strconv.Atoi(rawLabelValue)

	if err != nil {
		log.Printf("Provided label value %s should be of type uint", rawLabelValue)
		return fallback
	}

	return uint64(value)
}*/

