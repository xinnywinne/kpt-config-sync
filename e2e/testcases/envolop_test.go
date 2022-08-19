package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"kpt.dev/configsync/e2e/nomostest"
	"kpt.dev/configsync/e2e/nomostest/ntopts"
	"kpt.dev/configsync/pkg/api/configsync"
	"kpt.dev/configsync/pkg/client/restconfig"
	"kpt.dev/configsync/pkg/core"
	"kpt.dev/configsync/pkg/metadata"
	"kpt.dev/configsync/pkg/testing/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestPerfConfigMap1N(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "ThresholdTestName"
	labelValue := "TestThressholdConfigMap"
	objNum := 10000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)

	testName := "ConfigMap10K1N"
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.RootRepos[configsync.RootSyncName].Add("acme/ns.yaml", fake.NamespaceObject(
		"foo", core.Label(labelKey, labelValue)))
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/cm-%d.yaml", i), fake.ConfigMapObject(
			core.Name(fmt.Sprintf("cm%d", i)), core.Namespace("foo"), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Config Map", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	cmList := &corev1.ConfigMapList{}
	var wg sync.WaitGroup
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					if err := nt.Client.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue, labelKey: labelValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(cmList.Items))
					nt.T.Logf("ConfigMap at time %v %d", t, len(cmList.Items))
					if len(cmList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)

				}
			}
		}
	}()
	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	if len(cmList.Items) < objNum {
		nt.T.Error("Config Sync sync too slow")
	}
	wg.Wait()
	writeToFiles(testName, syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Config Map", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
	time.Sleep(4 * time.Minute)

}

func TestPerfConfigMap10Namespaces(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "ThresholdTestName"
	labelValue := "TestThressholdConfigMap"
	objNum := 10000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)

	testName := "ConfigMap10K10N"
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	for i := 0; i < 10; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/ns-%d.yaml", i), fake.NamespaceObject(
			fmt.Sprintf("foo%d", i), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	for i := 0; i < objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/cm-%d.yaml", i), fake.ConfigMapObject(
			core.Name(fmt.Sprintf("cm%d", i)), core.Namespace(fmt.Sprintf("foo%d", i%10)), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Config Map", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					cmList := &corev1.ConfigMapList{}
					if err := nt.Client.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue, labelKey: labelValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(cmList.Items))
					if len(cmList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)

				}
			}
		}
	}()
	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles(testName, syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 0; i < objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Config Map", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}

func TestPerfLargeConfigMap1N(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "StressTestName"
	labelValue := "TestStressCRD"
	objNum := 5000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.RootRepos[configsync.RootSyncName].Add("acme/ns.yaml", fake.NamespaceObject(
		"foo", core.Label(labelKey, labelValue)))
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].AddFile(fmt.Sprintf("acme/cm-%d.yaml", i), []byte(fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm%d
  namespace: foo
data:
  foo: |
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafafffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiap
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa`, i)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Large Config Map", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	testName := "LargeConfigMap5K1N"
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	cfg, err := restconfig.NewRestConfig(30 * time.Second)
	if err != nil {
		nt.T.Error(err)
	}
	cl, err := client.New(cfg, client.Options{})
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					cmList := &corev1.ConfigMapList{}
					if err := cl.List(nt.Context, cmList, client.InNamespace("foo"), client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(cmList.Items))
					nt.T.Logf("LargeConfigMap at time %v %d", t, len(cmList.Items))
					if len(cmList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)

				}
			}
		}
	}()
	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles(testName, syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Large Config Map", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}
func TestPerfLargeConfigMap10Namespaces(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "PerfTestName"
	labelValue := "TestPerfLargeConfigMap10N"
	objNum := 5000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	for i := 0; i < 10; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/ns-%d.yaml", i), fake.NamespaceObject(
			fmt.Sprintf("foo%d", i), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	for i := 0; i < objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].AddFile(fmt.Sprintf("acme/cm-%d.yaml", i), []byte(fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm%d
  namespace: foo%d
data:
  foo: |
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafafffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiapf
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa
    aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaafaffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
    fffffffffffffffffffffffffffffffffffffffffffffffffffffff
    ddddddddddddddffeeiaraehapeifhapeofihapeoifhapefhapeofhiap
    aefaeoifhapoeifhapeihapihaporhgpahgapohfapfhaefhafa`, i, i%10)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Large Config Map", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	testName := "LargeConfigMap5K10N"
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	cfg, err := restconfig.NewRestConfig(30 * time.Second)
	if err != nil {
		nt.T.Error(err)
	}
	cl, err := client.New(cfg, client.Options{})
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					cmList := &corev1.ConfigMapList{}
					if err := cl.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(cmList.Items))
					nt.T.Logf("LargeConfigMap at time %v %d", t, len(cmList.Items))
					if len(cmList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)
				}
			}
		}
	}()
	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles(testName, syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 0; i < objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Large Config Map", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}
func TestPerfDeployment(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Deployment time test start")
	nomostest.StopWebhook(nt)
	labelKey := "ThresholdTestName"
	labelValue := "TestThresholdDeploymentTime"
	objNum := 3000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.RootRepos[configsync.RootSyncName].Add("acme/ns.yaml", fake.NamespaceObject(
		"foo", core.Label(labelKey, labelValue)))
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].AddFile(fmt.Sprintf("acme/deployment%d.yaml", i), []byte(fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx%d
  namespace: foo
spec:
  replicas: 0
  minReadySeconds: 9
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - image: nginx:1.7.9
        name: nginx%d
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 80
          protocol: TCP
`, i, i)))

	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Deployments", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	testName := "Deployment3K"
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					dpList := &unstructured.UnstructuredList{}
					dpList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "apps",
						Kind:    "Deployment",
						Version: "v1",
					})

					if err := nt.Client.List(nt.Context, dpList, client.InNamespace("foo")); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					nt.T.Logf("Deployment at time %v %d", t, len(dpList.Items))
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(dpList.Items))
					if len(dpList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)
				}
			}
		}
	}()

	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles("Deployment3K", syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/deployment%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Deployments", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}

func TestPerfCR(t *testing.T) {
	time.Sleep(10 * time.Minute)
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	crdName := "crontabs.stable.example.com"
	labelKey := "ThresholdCR"
	labelValue := "TestThresholdCR"
	objNum := 3000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.T.Logf("Delete the %q CRD if needed", crdName)
	nt.MustKubectl("delete", "crd", crdName, "--ignore-not-found")
	crdContent, err := ioutil.ReadFile("../testdata/customresources/changed_schema_crds/old_schema_crd.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/ns-%d.yaml", i), fake.NamespaceObject(
			fmt.Sprintf("n%d", i), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d namespaces", objNum))
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/crontab-crd.yaml", crdContent)
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add CronTab CRD")
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	nt.T.Logf("Verify that the CronTab CRD is installed on the cluster")
	if err := nt.Validate(crdName, "", fake.CustomResourceDefinitionV1Object()); err != nil {
		nt.T.Fatal(err)
	}

	for i := 1; i <= objNum; i++ {
		cr, err := crontabCR(fmt.Sprintf("n%d", i), fmt.Sprintf("cr%d", i))
		if err != nil {
			nt.T.Fatal(err)
		}
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/crontab-cr-%d.yaml", i), cr)
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Crontab CRs", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	testName := "CR3K"
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					crList := &unstructured.UnstructuredList{}
					crList.SetGroupVersionKind(crontabGVK)
					if err := nt.Client.List(nt.Context, crList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					nt.T.Logf("CR at time %v %d", t, len(crList.Items))
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(crList.Items))

					if len(crList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("CR result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)
				}
			}
		}
	}()
	time.Sleep(5 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles("CR3K", syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/crontab-cr-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d CRs", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/ns-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Namespaces", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
	time.Sleep(4 * time.Minute)
}

func TestPerfLargeCR(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	crdName := "testgroups.test.dev"
	labelKey := "ThresholdCR"
	labelValue := "TestThresholdCR"
	objNum := 1000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.T.Logf("Delete the %q CRD if needed", crdName)
	nt.MustKubectl("delete", "crd", crdName, "--ignore-not-found")
	crdContent, err := ioutil.ReadFile("../testdata/resourcegroupcrd.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/ns-%d.yaml", i), fake.NamespaceObject(
			fmt.Sprintf("n%d", i), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d namespaces", objNum))
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/resourcegroup-crd.yaml", crdContent)
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add ResourceGroup CRD")
	nt.WaitForRepoSyncs()
	time.Sleep(5 * time.Minute)
	nt.T.Logf("Verify that the ResourceGroup CRD is installed on the cluster")
	if err := nt.Validate(crdName, "", fake.CustomResourceDefinitionV1Object()); err != nil {
		nt.T.Fatal(err)
	}
	filepath := "../testdata/resourcegroup.yaml"
	operatorManifest, err := ioutil.ReadFile(filepath)
	if err != nil {
		nt.T.Fatalf("failed to read file %s, %w", filepath, err)
	}
	obj := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, gvk, err := dec.Decode(operatorManifest, nil, obj)
	if err != nil {
		nt.T.Fatal(err)
	}
	for i := 1; i <= objNum; i++ {
		obj.SetNamespace(fmt.Sprintf("cr%d", i))
		obj.SetName(fmt.Sprintf("n%d", i))
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/resource-group-cr-%d.yaml", i), obj)
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d ResourceGroup CRs", objNum))
	syncTimeResult := ""
	timeResult := ""
	memCPUResult := ""
	testName := "LargeCR1K"
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 0
	var wg sync.WaitGroup
	cfg, err := restconfig.NewRestConfig(30 * time.Second)
	if err != nil {
		nt.T.Error(err)
	}
	cl, err := client.New(cfg, client.Options{})
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				{
					wg.Add(1)
					defer wg.Done()
					i++
					crList := &unstructured.UnstructuredList{}
					crList.SetGroupVersionKind(*gvk)
					if err := cl.List(nt.Context, crList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue}); err != nil {
						nt.T.Logf("Failed to get all resources: %v", err)
					}
					nt.T.Logf("CR at time %v %d", t, len(crList.Items))
					timeResult += fmt.Sprintf("%s %d %v\n", testName, 10*i, len(crList.Items))

					if len(crList.Items) == objNum && syncTimeResult == "" {
						syncTimeResult = fmt.Sprintf("%s %d\n", testName, 10*i)
						nt.T.Logf("CR result is %s", syncTimeResult)
					}
					memCPUResult += getMaxMemCPU(maxMem, maxCPU, nt, i*10, testName)
				}
			}
		}
	}()
	time.Sleep(5 * time.Minute)
	ticker.Stop()
	done <- true
	wg.Wait()
	writeToFiles(testName, syncTimeResult, timeResult, memCPUResult, maxMem, maxCPU, nt)
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/resource-group-cr-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d CRs", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(40 * time.Minute))
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/ns-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Namespaces", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(40 * time.Minute))
	time.Sleep(4 * time.Minute)
}

func TestMemoryConfigMap(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "ThresholdTestName"
	labelValue := "TestThresholdConfigMap"
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.MustKubectl("apply", "-f", filepath.Join(nt.RootRepos[configsync.RootSyncName].Root, "acme/metricsapi.yaml"))
	nt.RootRepos[configsync.RootSyncName].Add("acme/ns.yaml", fake.NamespaceObject(
		"foo", core.Label(labelKey, labelValue)))
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	for i := 1; i <= 10000; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/cm-%d.yaml", i), fake.ConfigMapObject(
			core.Name(fmt.Sprintf("cm%d", i)), core.Namespace("foo"), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add  10000 Config Map")
	result := ""
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	i := 1

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				{
					out, err := nt.Kubectl("top", "pod", "-n", "resource-group-system", "--containers")
					if err != nil {
						nt.T.Errorf("cannot get container memory usage, error: %v", err)
					}
					s := string(out)
					cmList := &corev1.ConfigMapList{}
					if err := nt.Client.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue, labelKey: labelValue}); err != nil {
						nt.T.Error(err)
					}

					sArray := strings.Split(s, "\n")
					for _, ss := range sArray {
						if strings.Contains(ss, "Mi") && strings.Contains(ss, "m ") {
							ns := strings.ReplaceAll(ss, "Mi", "")
							ns = strings.ReplaceAll(ns, "m ", "")
							ns = strings.Join(strings.Fields(ns), " ")
							result += "ConfigMap " + fmt.Sprintf("%v %d ", len(cmList.Items), i*10) + ns + "\n"
							nt.T.Logf("%d  %v", len(cmList.Items), ns)
						}

					}
					out, err = nt.Kubectl("top", "pod", "-n", "config-management-system", "--containers")
					if err != nil {
						nt.T.Errorf("cannot get container memory usage, error: %v", err)
					}
					s = string(out)
					cmList = &corev1.ConfigMapList{}
					if err := nt.Client.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue, labelKey: labelValue}); err != nil {
						nt.T.Error(err)
					}

					sArray = strings.Split(s, "\n")
					for _, ss := range sArray {
						if strings.Contains(ss, "Mi") && strings.Contains(ss, "m ") {
							ns := strings.ReplaceAll(ss, "Mi", "")
							ns = strings.ReplaceAll(ns, "m ", "")
							ns = strings.Join(strings.Fields(ns), " ")
							result += "ConfigMap " + fmt.Sprintf("%v %d ", len(cmList.Items), i*10) + ns + "\n"
							nt.T.Logf("%d  %v", len(cmList.Items), ns)
						}

					}
					i++
				}
			}
		}
	}()
	time.Sleep(8 * time.Minute)
	ticker.Stop()
	done <- true
	f, err := os.OpenFile("../testdata/perfdata/cm10k1n_memory.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(result); err != nil {
		nt.T.Fatal(err)
	}
	for i := 1; i <= 10000; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Remove 10000 Config Map")
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}

/*
func TestResultMemoryConfigMap(t *testing.T) {
	nt := nomostest.New(t, ntopts.Unstructured, ntopts.SkipMonoRepo)
	nt.T.Log("Stop the CS webhook by removing the webhook configuration")
	nomostest.StopWebhook(nt)
	labelKey := "ThresholdTestName"
	labelValue := "TestThresholdConfigMap"
	objNum := 1000
	maxMem := make(map[string]int)
	maxCPU := make(map[string]int)
	metricsContent, err := ioutil.ReadFile("../testdata/metricsapi.yaml")
	if err != nil {
		nt.T.Fatal(err)
	}
	nt.RootRepos[configsync.RootSyncName].AddFile("acme/metricsapi.yaml", metricsContent)
	nt.MustKubectl("apply", "-f", filepath.Join(nt.RootRepos[configsync.RootSyncName].Root, "acme/metricsapi.yaml"))
	nt.RootRepos[configsync.RootSyncName].Add("acme/ns.yaml", fake.NamespaceObject(
		"foo", core.Label(labelKey, labelValue)))
	nt.RootRepos[configsync.RootSyncName].CommitAndPush("Add namespace foo")
	rootSync := fake.RootSyncObjectV1Beta1(configsync.RootSyncName)
	nt.MustMergePatch(rootSync, `{"spec": {"override": {"statusMode": "disabled"}}}`)
	nt.WaitForRepoSyncs()
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Add(fmt.Sprintf("acme/cm-%d.yaml", i), fake.ConfigMapObject(
			core.Name(fmt.Sprintf("cm%d", i)), core.Namespace("foo"), core.Label(labelKey, labelValue)))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Add %d Config Map", objNum))
	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				{
					getMaxMemCPU(maxMem, maxCPU, nt)
				}
			}
		}
	}()
	time.Sleep(4 * time.Minute)
	ticker.Stop()
	done <- true
	cmList := &corev1.ConfigMapList{}
	if err := nt.Client.List(nt.Context, cmList, client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue, labelKey: labelValue}); err != nil {
		nt.T.Error(err)
	}
	if len(cmList.Items) < objNum {
		nt.T.Error("Config Sync sync too slow")
	}
	f, err := os.OpenFile("../testdata/perfdata/tt.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f.Close()
	for container, cpu := range maxCPU {
		fmt.Printf("!!!!!!!!!!!!!!!%s %d %d\n", container, cpu, maxMem[container])
		if _, err := f.WriteString(container + " " + fmt.Sprint(cpu) + " " + fmt.Sprint(maxMem[container]) + "\n"); err != nil {
			nt.T.Fatal(err)
		}
	}
	for i := 1; i <= objNum; i++ {
		nt.RootRepos[configsync.RootSyncName].Remove(fmt.Sprintf("acme/cm-%d.yaml", i))
	}
	nt.RootRepos[configsync.RootSyncName].CommitAndPush(fmt.Sprintf("Remove %d Config Map", objNum))
	nt.WaitForRepoSyncs(nomostest.WithTimeout(20 * time.Minute))
}
*/
func updateMemCPU(mem, cpu map[string]int, CPUIdx, MemIdx int, sin, podName string, seconds int, testName string) string {
	ss := strings.Fields(sin)
	cpuValue, _ := strconv.Atoi(strings.ReplaceAll(ss[CPUIdx], "m", ""))
	memValue, _ := strconv.Atoi(strings.ReplaceAll(ss[MemIdx], "Mi", ""))
	result := testName + " " + podName + " " + fmt.Sprint(seconds) + " " + fmt.Sprint(cpuValue) + " " + fmt.Sprint(memValue) + "\n"

	if cpu[podName] < cpuValue {
		cpu[podName] = cpuValue
		fmt.Printf("update cpu to be %d", cpuValue)
	}
	if mem[podName] < memValue {
		mem[podName] = memValue
		fmt.Printf("update mem to %d", memValue)
	}
	return result
}
func getMaxMemCPU(mem, cpu map[string]int, nt *nomostest.NT, seconds int, testName string) string {
	out, err := nt.Kubectl("top", "pod", "-n", "resource-group-system", "--containers")
	if err != nil {
		nt.T.Errorf("cannot get container memory usage, error: %v", err)
	}
	result := ""
	CPUIdx := 0
	MemIdx := 0
	sArray := strings.Split(string(out), "\n")
	for _, s := range sArray {
		if strings.Contains(s, "resource-group-controller") {
			if strings.Contains(s, "kube-rbac-proxy") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "resource-group-controller-kube-rbac-proxy", seconds, testName)
			} else if strings.Contains(s, "otel-agent") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "resource-group-controller-otel-agent", seconds, testName)
			} else {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "resource-group-controller-manager", seconds, testName)
			}

		} else if strings.Contains(s, "CPU") {
			ss := strings.Fields(s)
			for i, str := range ss {
				if strings.Contains(str, "CPU") {
					CPUIdx = i
				} else if strings.Contains(str, "MEMORY") {
					MemIdx = i
				}
			}
		}
	}

	out, err = nt.Kubectl("top", "pod", "-n", "config-management-system", "--containers")
	if err != nil {
		nt.T.Errorf("cannot get container memory usage, error: %v", err)
	}
	sArray = strings.Split(string(out), "\n")
	for _, s := range sArray {
		if strings.Contains(s, "reconciler-manager") {
			if strings.Contains(s, "otel-agent") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-manager-otel-agent", seconds, testName)
			} else if strings.Contains(s, " reconciler-manager ") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-manager-reconciler-manager", seconds, testName)
			}
		} else if strings.Contains(s, "root-reconciler") {
			if strings.Contains(s, "otel-agent") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-otel-agent", seconds, testName)
			} else if strings.Contains(s, "git-sync") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-git-sync", seconds, testName)
			} else if strings.Contains(s, "hydration-controller") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-hydration-controller", seconds, testName)
			} else if strings.Contains(s, " reconciler ") {
				result += updateMemCPU(mem, cpu, CPUIdx, MemIdx, s, "reconciler-reconciler", seconds, testName)
			}
		}
	}
	return result
}

func writeToFiles(testName, maxTime, time, memCPU string, maxMem, maxCPU map[string]int, nt *nomostest.NT) {
	f1, err := os.OpenFile(fmt.Sprintf("../testdata/perfdata/1_max_time_%s.txt", testName),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f1.Close()
	if _, err := f1.WriteString(maxTime); err != nil {
		nt.T.Fatal(err)
	}
	f2, err := os.OpenFile(fmt.Sprintf("../testdata/perfdata/2_max_memcpu_%s.txt", testName),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f2.Close()
	for container, cpu := range maxCPU {
		fmt.Printf("!!!!!!!!!!!!!!!%s %d %d\n", container, cpu, maxMem[container])
		if _, err := f2.WriteString(container + " " + fmt.Sprint(cpu) + " " + fmt.Sprint(maxMem[container]) + "\n"); err != nil {
			nt.T.Fatal(err)
		}
	}
	f3, err := os.OpenFile(fmt.Sprintf("../testdata/perfdata/3_time_%s.txt", testName),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f3.Close()
	if _, err := f3.WriteString(time); err != nil {
		nt.T.Fatal(err)
	}
	f4, err := os.OpenFile(fmt.Sprintf("../testdata/perfdata/4_memcpu_%s.txt", testName),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		nt.T.Fatal(err)
	}
	defer f4.Close()
	if _, err := f4.WriteString(memCPU); err != nil {
		nt.T.Fatal(err)
	}
}
