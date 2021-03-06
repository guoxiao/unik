package main

import (
	"fmt"
	"os"

	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/emc-advanced-dev/unik/pkg/compilers"
	"github.com/emc-advanced-dev/unik/pkg/config"
	unikos "github.com/emc-advanced-dev/unik/pkg/os"
	"github.com/emc-advanced-dev/unik/pkg/providers/aws"
	"github.com/emc-advanced-dev/unik/pkg/state"
	uniklog "github.com/emc-advanced-dev/unik/pkg/util/log"
	"strings"
)

func main() {
	action := flag.String("action", "all", "what to test")
	arg := flag.String("arg", "", "option for some test (i.e. instance id)")
	flag.Parse()
	os.Setenv("TMPDIR", os.Getenv("HOME")+"/tmp/uniktest")
	logrus.SetLevel(logrus.DebugLevel)
	logrus.AddHook(&uniklog.AddTraceHook{true})

	c := config.Aws{
		Name:              "aws-provider",
		AwsAccessKeyID:    os.Getenv("AWS_ACCESS_KEY_ID"),
		AwsSecretAcessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Region:            os.Getenv("AWS_REGION"),
		Zone:              os.Getenv("AWS_AVAILABILITY_ZONE"),
	}
	p := aws.NewAwsProvier(c)
	state, err := state.BasicStateFromFile(aws.AwsStateFile)
	if err != nil {
		logrus.WithError(err).Error("failed to load state")
	} else {
		logrus.Info("state loaded")
		p = p.WithState(state)
	}

	switch *action {
	case "all":
		r := compilers.RumpCompiler{
			DockerImage: "projectunik/compilers-rump-go-xen",
			CreateImage: compilers.CreateImageAws,
		}
		f, err := os.Open("a.tar")
		if err != nil {
			logrus.Error(err)
			return
		}
		rawimg, err := r.CompileRawImage(f, "", []string{})
		if err != nil {
			logrus.Error(err)
			return
		}

		img, err := p.Stage("test-scott", rawimg, true)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("image", img).Infof("printing image")
		fmt.Println()

		env := make(map[string]string)
		env["FOO"] = "BAR"

		instance, err := p.RunInstance("test-scott-instance-1", img.Id, nil, env)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("instance", instance).Infof("printing instance")
		fmt.Println()

		images, err := p.ListImages()
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("images", images).Infof("printing images")
		fmt.Println()

		instances, err := p.ListInstances()
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("instances", instances).Infof("printing instances")
		fmt.Println()

		for _, instance := range instances {
			if err := p.DeleteInstance(instance.Id); err != nil {
				logrus.Error(err)
				return
			}
		}

		for _, image := range images {
			if err := p.DeleteImage(image.Id, false); err != nil {
				logrus.Error(err)
				return
			}
		}
		break
	case "list-images":
		images, err := p.ListImages()
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("images", images).Infof("printing images")
		break
	case "list-volumes":
		volumes, err := p.ListVolumes()
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("volumes", volumes).Infof("printing volumes")
		break
	case "list-instances":
		instances, err := p.ListInstances()
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("instances", instances).Infof("printing instances")
		break
	case "delete-instance":
		instanceId := *arg
		if err := p.DeleteInstance(instanceId); err != nil {
			logrus.Error(err)
			return
		}
		logrus.Infof("deleted instance %s", instanceId)
		break
	case "create-image":
		r := compilers.RumpCompiler{
			DockerImage: "projectunik/compilers-rump-go-xen",
			CreateImage: compilers.CreateImageAws,
		}
		f, err := os.Open("a.tar")
		if err != nil {
			logrus.Error(err)
			return
		}
		rawimg, err := r.CompileRawImage(f, "", []string{})
		if err != nil {
			logrus.Error(err)
			return
		}

		img, err := p.Stage("test-scott", rawimg, true)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("image", img).Infof("printing image")
		break
	case "create-image-with-volume":
		name := *arg
		r := compilers.RumpCompiler{
			DockerImage: "projectunik/compilers-rump-go-xen",
			CreateImage: compilers.CreateImageAws,
		}
		f, err := os.Open("a.tar")
		if err != nil {
			logrus.Error(err)
			return
		}
		rawimg, err := r.CompileRawImage(f, "", []string{"/data"})
		if err != nil {
			logrus.Error(err)
			return
		}

		img, err := p.Stage(name, rawimg, true)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("image", img).Infof("printing image")
		break
	case "delete-image":
		imageId := *arg
		if err := p.DeleteImage(imageId, true); err != nil {
			logrus.Error(err)
			return
		}
		logrus.Infof("deleted image %s", imageId)
		break
	case "create-volume":
		name := *arg
		f, err := os.Open("a.tar")
		if err != nil {
			logrus.Error(err)
			return
		}
		imagePath, err := unikos.BuildRawDataImage(f, 0, false)
		if err != nil {
			logrus.Error(err)
			return
		}
		defer os.RemoveAll(imagePath)
		logrus.Infof("built raw image %s", imagePath)
		volume, err := p.CreateVolume(name, imagePath)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("volume", volume).Infof("created volume %s", name)
		break
	case "run-instance":
		name := strings.Split(*arg, ",")[0]
		imageName := strings.Split(*arg, ",")[1]

		instance, err := p.RunInstance(name, imageName, nil, nil)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("instance", instance).Infof("instance %s", name)
		break
	case "get-instance":
		instance, err := p.GetInstance(*arg)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("instance", instance).Infof("instance %s", *arg)
		break
	case "run-instance-with-volume":
		args := strings.Split(*arg, ",")
		if len(args) != 4 {
			logrus.Error("wrong args: " + *arg)
			return
		}
		name := args[0]
		imageName := args[1]
		mntPoint := args[2]
		volumeId := args[3]
		mntsToVols := map[string]string{
			mntPoint: volumeId,
		}
		instance, specErr := p.RunInstance(name, imageName, mntsToVols, nil)
		if specErr != nil {
			logrus.Error(specErr)
			return
		}
		volume, err := p.GetVolume(volumeId)
		if err != nil {
			logrus.Error(err)
			return
		}
		updatedInstance, err := p.GetInstance(instance.Id)
		if err != nil {
			logrus.Error(err)
			return
		}
		logrus.WithField("volume", volume).Infof("attached volume")
		logrus.WithField("updatedInstance", updatedInstance).Infof("updatedInstance %s", name)
		break
	}

}
