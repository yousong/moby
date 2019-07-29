package daemon

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container"
	"github.com/docker/docker/errors"
	"github.com/docker/docker/layer"
	volumestore "github.com/docker/docker/volume/store"
	"github.com/docker/engine-api/types"
)

var cgroupFilePaths = map[string]string{"cpu,cpuacct":"/sys/fs/cgroup/cpu,cpuacct/docker/%s",
	"net_cls,net_prio":"/sys/fs/cgroup/net_cls,net_prio/docker/%s",
	"cpuset":"/sys/fs/cgroup/cpuset/docker/%s",
	"freezer":"/sys/fs/cgroup/freezer/docker/%s",
	"memory":"/sys/fs/cgroup/memory/docker/%s",
	"systemd":"/sys/fs/cgroup/systemd/docker/%s",
	"net_cls":"/sys/fs/cgroup/net_cls/docker/%s",
	"blkio":"/sys/fs/cgroup/blkio/docker/%s",
	"cpu":"/sys/fs/cgroup/cpu/docker/%s",
	"cpuacct":"/sys/fs/cgroup/cpuacct/docker/%s",
	"devices":"/sys/fs/cgroup/devices/docker/%s",
	"hugetlb":"/sys/fs/cgroup/hugetlb/docker/%s",
	"net_prio":"/sys/fs/cgroup/net_prio/docker/%s",
	"perf_event":"/sys/fs/cgroup/perf_event/docker/%s",
	"pids":"/sys/fs/cgroup/pids/docker/%s"}

// removePaths iterates over the provided paths removing them.
// We trying to remove all paths five times with increasing delay between tries.
// If after all there are not removed cgroups - appropriate error will be
// returned.
func removePaths(paths map[string]string) (err error) {
	delay := 10 * time.Millisecond
	for i := 0; i < 5; i++ {
		if i != 0 {
			time.Sleep(delay)
			delay *= 2
		}
		for s, p := range paths {
			os.RemoveAll(p)
			//os.RemoveAll(p)
			// TODO: here probably should be logging
			_, err := os.Stat(p)
			// We need this strange way of checking cgroups existence because
			// RemoveAll almost always returns error, even on already removed
			// cgroups
			if os.IsNotExist(err) {
				delete(paths, s)
			}
		}
		if len(paths) == 0 {
			return nil
		}
	}
	return fmt.Errorf("Failed to remove paths: %v", paths)
}

// ContainerRm removes the container id from the filesystem. An error
// is returned if the container is not found, or if the remove
// fails. If the remove succeeds, the container name is released, and
// network links are removed.
func (daemon *Daemon) ContainerRm(name string, config *types.ContainerRmConfig) error {
	container, err := daemon.GetContainer(name)
	if err != nil {
		return err
	}

	// Container state RemovalInProgress should be used to avoid races.
	if inProgress := container.SetRemovalInProgress(); inProgress {
		return nil
	}
	defer container.ResetRemovalInProgress()

	// check if container wasn't deregistered by previous rm since Get
	if c := daemon.containers.Get(container.ID); c == nil {
		return nil
	}

	if config.RemoveLink {
		return daemon.rmLink(container, name)
	}

	err = daemon.cleanupContainer(container, config.ForceRemove)
	if err == nil || config.ForceRemove {
		if e := daemon.removeMountPoints(container, config.RemoveVolume); e != nil {
			logrus.Error(e)
		}
	}

	return err
}

func (daemon *Daemon) rmLink(container *container.Container, name string) error {
	if name[0] != '/' {
		name = "/" + name
	}
	parent, n := path.Split(name)
	if parent == "/" {
		return fmt.Errorf("Conflict, cannot remove the default name of the container")
	}

	parent = strings.TrimSuffix(parent, "/")
	pe, err := daemon.nameIndex.Get(parent)
	if err != nil {
		return fmt.Errorf("Cannot get parent %s for name %s", parent, name)
	}

	daemon.releaseName(name)
	parentContainer, _ := daemon.GetContainer(pe)
	if parentContainer != nil {
		daemon.linkIndex.unlink(name, container, parentContainer)
		if err := daemon.updateNetwork(parentContainer); err != nil {
			logrus.Debugf("Could not update network to remove link %s: %v", n, err)
		}
	}
	return nil
}

// cleanupContainer unregisters a container from the daemon, stops stats
// collection and cleanly removes contents and metadata from the filesystem.
func (daemon *Daemon) cleanupContainer(container *container.Container, forceRemove bool) (err error) {
	if container.IsRunning() {
		if !forceRemove {
			err := fmt.Errorf("You cannot remove a running container %s. Stop the container before attempting removal or use -f", container.ID)
			return errors.NewRequestConflictError(err)
		}
		if err := daemon.Kill(container); err != nil {
			return fmt.Errorf("Could not kill running container %s, cannot remove - %v", container.ID, err)
		}
	}

	// stop collection of stats for the container regardless
	// if stats are currently getting collected.
	daemon.statsCollector.stopCollection(container)

	if err = daemon.containerStop(container, 3); err != nil {
		return err
	}

	// Mark container dead. We don't want anybody to be restarting it.
	container.SetDead()

	// Save container state to disk. So that if error happens before
	// container meta file got removed from disk, then a restart of
	// docker should not make a dead container alive.
	if err := container.ToDiskLocking(); err != nil && !os.IsNotExist(err) {
		logrus.Errorf("Error saving dying container to disk: %v", err)
	}

	// If force removal is required, delete container from various
	// indexes even if removal failed.
	defer func() {
		if err == nil || forceRemove {
			daemon.nameIndex.Delete(container.ID)
			daemon.linkIndex.delete(container)
			selinuxFreeLxcContexts(container.ProcessLabel)
			daemon.idIndex.Delete(container.ID)
			daemon.containers.Delete(container.ID)
			daemon.LogContainerEvent(container, "destroy")
		}
	}()

	if err = os.RemoveAll(container.Root); err != nil {
		return fmt.Errorf("Unable to remove filesystem for %v: %v", container.ID, err)
	}

	// When container creation fails and `RWLayer` has not been created yet, we
	// do not call `ReleaseRWLayer`
	if container.RWLayer != nil {
		metadata, err := daemon.layerStore.ReleaseRWLayer(container.RWLayer)
		layer.LogReleaseMetadata(metadata)
		if err != nil && err != layer.ErrMountDoesNotExist {
			return fmt.Errorf("Driver %s failed to remove root filesystem %s: %s", daemon.GraphDriverName(), container.ID, err)
		}
	}

	// destroy cgroup files
	cgroupPaths := make(map[string]string)
	for s, p := range cgroupFilePaths {
		cgroupPaths[s] = fmt.Sprintf(p, container.ID)
	}

	err = removePaths(cgroupPaths)
	if err != nil {
		return fmt.Errorf("Fail to destroy cgroups of container %s, err %s", container.ID, err)
	}

	return nil
}

// VolumeRm removes the volume with the given name.
// If the volume is referenced by a container it is not removed
// This is called directly from the remote API
func (daemon *Daemon) VolumeRm(name string) error {
	v, err := daemon.volumes.Get(name)
	if err != nil {
		return err
	}

	if err := daemon.volumes.Remove(v); err != nil {
		if volumestore.IsInUse(err) {
			err := fmt.Errorf("Unable to remove volume, volume still in use: %v", err)
			return errors.NewRequestConflictError(err)
		}
		return fmt.Errorf("Error while removing volume %s: %v", name, err)
	}
	daemon.LogVolumeEvent(v.Name(), "destroy", map[string]string{"driver": v.DriverName()})
	return nil
}
