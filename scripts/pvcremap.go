package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const replicationPrefix = "replication.storage.dell.com/"

func main() {
	fmt.Printf("version 21\n")

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	rgNamePtr := flag.String("rg", "", "id for the RG")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubeconfig: %s\n", err.Error())
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating clientset: %s\n", err.Error())
		os.Exit(1)
	}

	if rgNamePtr == nil {
		fmt.Printf("PVC name is required")
		os.Exit(1)
	}

	rgName := *rgNamePtr
	ctx := context.TODO()
	err = swapAllPVC(ctx, clientset, rgName)

}

func swapAllPVC(ctx context.Context, clientset *kubernetes.Clientset, rgName string) error {
	fmt.Printf("calling getPVList from %s\n", rgName)

	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"replication.storage.dell.com/replicationGroupName": rgName}}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}

	pvcs, err := clientset.CoreV1().PersistentVolumeClaims("").List(ctx, listOptions)

	if err != nil {
		return fmt.Errorf("failed to list PVCs: %w", err)
	}
	for _, pvc := range pvcs.Items {
		pvcName := pvc.Name
		namespace := pvc.Namespace
		targetPV := pvc.Annotations[replicationPrefix+"remotePV"]
		err = swapPVC(ctx, clientset, pvcName, namespace, targetPV)
	}
	return err
}

func swapPVC(ctx context.Context, clientset *kubernetes.Clientset, pvcName, namespace, targetPV string) error {
	fmt.Printf("pvc %s/%s targetPV %s\n", namespace, pvcName, targetPV)

	// Read the PVC
	pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		logf("Error getting pv %s: %s:, pvcName, err)")
	}

	// Save the Reclaim Policy for both PVs - return reclaim policy to makepvcreclaimpolicyretain
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving local PV %s: %s", pvc.Spec.VolumeName, err.Error())
		return err
	}
	localPVPolicy := pv.Spec.PersistentVolumeReclaimPolicy
	logf("Saving reclaim policy of local PV: %s\n", string(localPVPolicy))

	pv, err = clientset.CoreV1().PersistentVolumes().Get(ctx, pvc.Annotations[replicationPrefix+"remotePV"], metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving remote PV %s: %s", pvc.Annotations[replicationPrefix+"remotePV"], err.Error())
		return err
	}
	remotePVPolicy := pv.Spec.PersistentVolumeReclaimPolicy
	logf("Saving reclaim policy of remote PV: %s\n", string(remotePVPolicy))

	// Make the PVS reclaim policy Retain
	if err = makePVReclaimPolicyRetain(ctx, clientset, pvc.Spec.VolumeName); err != nil {
		return err
	}

	if err = makePVReclaimPolicyRetain(ctx, clientset, pvc.Annotations[replicationPrefix+"remotePV"]); err != nil {
		return err
	}

	// Check that the request targetPV volume is our replia (remotePV)
	remotePV := pvc.Annotations[replicationPrefix+"remotePV"]
	if targetPV != remotePV {
		err := fmt.Errorf("requested target %s doesn't match available replica %s", targetPV, remotePV)
		logf(err.Error())
		return err
	}

	// Delete the existing PVC
	logf("Deleting PVC %s\n", pvcName)
	err = clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, pvcName, metav1.DeleteOptions{})
	if err != nil {
		logf("error deleting PVC %s: %s", pvcName, err.Error())
		return err
	}

	// Wait until PVC is deleted
	done := false
	for iteration := 0; !done; iteration++ {
		time.Sleep(1 * time.Second)
		_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				done = true
			}
		}
		if iteration > 30 {
			return fmt.Errorf("timed out waiting on PVC %s/%s to be deleted", namespace, pvcName)
		}
	}

	// Swap some fields in the PVC.
	//localPV := pvc.Annotations[replicationPrefix+"remotePV"]
	localPV := pvc.Spec.VolumeName
	pvc.Annotations[replicationPrefix+"remotePV"] = pvc.Spec.VolumeName
	pvc.Spec.VolumeName = remotePV

	remoteStorageClassName := pvc.Annotations[replicationPrefix+"remoteStorageClassName"]
	pvc.Annotations[replicationPrefix+"remoteStorageClassName"] = *pvc.Spec.StorageClassName
	pvc.Spec.StorageClassName = &remoteStorageClassName
	pvc.ObjectMeta.ResourceVersion = ""

	// Re-create the PVC, now pointing to the target.
	fmt.Printf("printing final PVC: %+v\n", pvc)
	logf("Recreating PVC %s", pvc.Name)
	pvc, err = clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Error creating final PVC: %s\n", err.Error())
		return err
	}

	// Update the target PV's claimref to point to our pvc.
	pvcUid := pvc.ObjectMeta.UID
	pvcResourceVersion := pvc.ObjectMeta.ResourceVersion
	err = updatePVClaimRef(ctx, clientset, targetPV, pvc.Namespace, pvcResourceVersion, pvc.Name, pvcUid)
	if err != nil {
		return err
	}

	// Verify pvc is created and bound to new PVs
	// remotePV is the current localPVName arg, localPV is the current remotePVName arg
	err = verifyPVC(ctx, clientset, remotePV, localPV, pvcName, namespace)

	if err == nil {
		// Remove the PVC reclaim of local PV
		removePVClaimRef(ctx, clientset, localPV)
		// Restore the PVs original volume reclaim policy
		setPVReclaimPolicy(ctx, clientset, pvc.Spec.VolumeName, remotePVPolicy)
		setPVReclaimPolicy(ctx, clientset, pvc.Annotations[replicationPrefix+"remotePV"], localPVPolicy)
	} else {
		logf("Error creating and binding PVC to new PVs %s and %s", localPV, remotePV)
	}

	fmt.Println("Operation completed successfully")
	return nil
}

func verifyPVC(ctx context.Context, clientset *kubernetes.Clientset, localPVName string, remotePVName string, pvcName string, namespace string) error {
	logf("Verifying")
	done := false
	for iteration := 0; !done; iteration++ {
		time.Sleep(1 * time.Second)
		pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
		if (err == nil) && (localPVName == pvc.Spec.VolumeName) && (remotePVName == pvc.Annotations[replicationPrefix+"remotePV"]) {
			done = true
			return err
		}

		if iteration > 30 {
			return fmt.Errorf("timed out waiting on PVC %s/%s to be created and bound", namespace, pvcName)
		}
	}
	return nil
}

func setPVReclaimPolicy(ctx context.Context, clientset *kubernetes.Clientset, pvName string, prevPolicy v1.PersistentVolumeReclaimPolicy) error {
	logf("Updating reclaim policy to previous policy on PV %s: ", pvName)
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving PV %s: %s", pvName, err.Error())
		return err
	}
	done := false
	for iterations := 0; !done; iterations++ {
		time.Sleep(2 * time.Second)
		pv, err = clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			logf("Error retrieving PV %s: %s", pvName, err.Error())
			return err
		}
		pv.Spec.PersistentVolumeReclaimPolicy = prevPolicy
		pv, err = clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
		if err != nil {
			logf("Error updating PV %s: %s", pvName, err.Error())
		}
		if pv.Spec.PersistentVolumeReclaimPolicy == prevPolicy {
			done = true
		} else if iterations > 20 {
			err := fmt.Errorf("Timed out waiting on PV VolumeReclaimPolicy to be set to previous policy")
			return err
		}
	}
	logf("Updating reclaim policy to previous completed on PV, now restored to: %s ", string(prevPolicy))
	return err
}

func makePVReclaimPolicyRetain(ctx context.Context, clientset *kubernetes.Clientset, pvName string) error {
	logf("Updating reclaim policy to Retain on PV")
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving PV %s: %s", pvName, err.Error())
		return err
	}
	pv.Spec.PersistentVolumeReclaimPolicy = "Retain"
	pv, err = clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
	if err != nil {
		logf("Error updating PV %s: %s", pvName, err.Error())
	}
	done := false
	for iterations := 0; !done; iterations++ {
		time.Sleep(2 * time.Second)
		pv, err = clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			logf("Error retrieving PV %s: %s", pvName, err.Error())
			return err
		}
		if pv.Spec.PersistentVolumeReclaimPolicy == "Retain" {
			done = true
		} else if iterations > 20 {
			err := fmt.Errorf("Timed out waiting on PV VolumeReclaimPolicy to be set to Retain")
			return err
		}
	}
	logf("Updating reclaim policy to Retain completed on PV")
	return err
}

func removePVClaimRef(ctx context.Context, clientset *kubernetes.Clientset, pvName string) error {
	logf("Removing ClaimRef on LocalPV: %s", pvName)
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving PV %s: %s", pvName, err.Error())
		return err
	}
	if pv.Spec.ClaimRef != nil {
		logf("finding claimref under pvc: %s", pv.Spec.ClaimRef.Name)
	}
	pv.Spec.ClaimRef = nil
	done := false
	for iterations := 0; !done; iterations++ {
		time.Sleep(2 * time.Second)
		_, err = clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
		pv, err = clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			logf("Error retrieving PV %s: %s", pvName, err.Error())
			return err
		}
		_, err = clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
		if err == nil {
			done = true
		} else if iterations > 20 {
			err := fmt.Errorf("Timed out waiting on Local PV Claim Ref to be remove")
			return err
		}
	}

	if err != nil {
		logf("Error updating PV %s: %s", pvName, err.Error())
	}
	if pv.Spec.ClaimRef != nil {
		logf("Error updating, finding claimref under pvc: %s", pv.Spec.ClaimRef.Name)
	}
	return err
}

// updatePVClaimRef updates the PV's ClaimRef.Uid to the specified value
func updatePVClaimRef(ctx context.Context, clientset *kubernetes.Clientset, pvName, pvcNamespace, pvcResourceVersion, pvcName string, pvcUid types.UID) error {
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		logf("Error retrieving PV %s: %s", pvName, err.Error())
		return err
	}
	if pv.Spec.ClaimRef == nil {
		pv.Spec.ClaimRef = &v1.ObjectReference{}
	}
	pv.Spec.ClaimRef.Kind = "PersistentVolumeClaim"
	pv.Spec.ClaimRef.Namespace = pvcNamespace
	pv.Spec.ClaimRef.Name = pvcName
	pv.Spec.ClaimRef.UID = pvcUid
	pv.Spec.ClaimRef.ResourceVersion = pvcResourceVersion

	done := false
	for iterations := 0; !done; iterations++ {
		time.Sleep(2 * time.Second)
		pv, err = clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
		if err != nil {
			logf("Error retrieving PV %s: %s", pvName, err.Error())
			return err
		}
		_, err = clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
		if err == nil {
			done = true
		} else if iterations > 20 {
			err := fmt.Errorf("Timed out waiting on PV VolumeReclaimPolicy to be set to Retain")
			return err
		}
	}

	if err != nil {
		logf("Error updating PV %s: %s", pvName, err.Error())
	}
	return err
}

func makePVClaimRefUid(ctx context.Context, clientset *kubernetes.Clientset, pvName, uid string) {
	logf("Updating PV %s claimRef to %s", pvName, uid)

}

func logf(format string, vars ...string) {
	fmt.Printf(format, vars)
	fmt.Printf("\n")
}
