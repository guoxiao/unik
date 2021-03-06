package virtualbox

import (
	"github.com/emc-advanced-dev/unik/pkg/providers/common"
	"github.com/emc-advanced-dev/unik/pkg/providers/virtualbox/virtualboxclient"
	"github.com/emc-advanced-dev/unik/pkg/types"
	"github.com/emc-advanced-dev/pkg/errors"
	"github.com/Sirupsen/logrus"
	"github.com/emc-advanced-dev/unik/pkg/compilers"
)

func (p *VirtualboxProvider) AttachVolume(id, instanceId, mntPoint string) error {
	volume, err := p.GetVolume(id)
	if err != nil {
		return errors.New("retrieving volume "+id, err)
	}
	instance, err := p.GetInstance(instanceId)
	if err != nil {
		return errors.New("retrieving instance "+instanceId, err)
	}
	image, err := p.GetImage(instance.ImageId)
	if err != nil {
		return errors.New("retrieving image for instance", err)
	}
	controllerPort, err := common.GetControllerPortForMnt(image, mntPoint)
	if err != nil {
		return errors.New("getting controller port for mnt point", err)
	}
	storageType := getStorageType(image.ExtraConfig)
	logrus.Debugf("using storage controller %s", storageType)

	switch storageType {
	case compilers.SCSI_Storage:
		if err := virtualboxclient.AttachDiskSCSI(instance.Id, getVolumePath(volume.Name), controllerPort); err != nil {
			return errors.New("attaching scsi disk to vm", err)
		}
	case compilers.SATA_Storage:
		if err := virtualboxclient.AttachDiskSATA(instance.Id, getVolumePath(volume.Name), controllerPort); err != nil {
			return errors.New("attaching sata disk to vm", err)
		}
	default:
		return errors.New("unknown storage type: "+string(storageType), nil)
	}

	if err := p.state.ModifyVolumes(func(volumes map[string]*types.Volume) error {
		volume, ok := volumes[volume.Id]
		if !ok {
			return errors.New("no record of "+volume.Id+" in the state", nil)
		}
		volume.Attachment = instance.Id
		return nil
	}); err != nil {
		return errors.New("modifying volumes in state", err)
	}
	if err := p.state.Save(); err != nil {
		return errors.New("saving instance volume map to state", err)
	}
	return nil
}
