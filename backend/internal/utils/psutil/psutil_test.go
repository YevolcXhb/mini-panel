package psutil

import (
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestSelectPhysicalPartitions(t *testing.T) {
	partitions := []disk.PartitionStat{
		{Device: "/dev/mmcblk0p1", Mountpoint: "/", Fstype: "f2fs"},
		{Device: "/dev/mmcblk0p1", Mountpoint: "/data", Fstype: "f2fs"},
		{Device: "/dev/mmcblk0p1", Mountpoint: "/data/sdcard", Fstype: "f2fs"},
		{Device: "/dev/mmcblk1p1", Mountpoint: "/sdcard", Fstype: "ext4"},
		{Device: "overlay", Mountpoint: "/var/lib/docker/overlay2/abc", Fstype: "overlay"},
		{Device: "tmpfs", Mountpoint: "/dev/shm", Fstype: "tmpfs"},
	}

	got := selectPhysicalPartitions(partitions)
	want := []string{"/", "/sdcard"}
	if len(got) != len(want) {
		t.Fatalf("got %d partitions, want %d: %+v", len(got), len(want), got)
	}
	for i, mp := range want {
		if got[i].Mountpoint != mp {
			t.Errorf("partition[%d] mountpoint = %q, want %q", i, got[i].Mountpoint, mp)
		}
	}
}

func TestSelectPhysicalPartitionsPrefersRoot(t *testing.T) {
	partitions := []disk.PartitionStat{
		{Device: "/dev/root", Mountpoint: "/data", Fstype: "f2fs"},
		{Device: "/dev/root", Mountpoint: "/", Fstype: "f2fs"},
	}

	got := selectPhysicalPartitions(partitions)
	if len(got) != 1 || got[0].Mountpoint != "/" {
		t.Fatalf("expected only root mountpoint, got %+v", got)
	}
}

func TestSelectPhysicalPartitionsKeepsEmptyDevice(t *testing.T) {
	partitions := []disk.PartitionStat{
		{Device: "", Mountpoint: "/mnt/nfs", Fstype: "nfs4"},
		{Device: "", Mountpoint: "/mnt/cifs", Fstype: "cifs"},
	}

	got := selectPhysicalPartitions(partitions)
	if len(got) != 2 {
		t.Fatalf("expected both network mounts to be kept, got %+v", got)
	}
}

func TestSelectPhysicalPartitionsSkipsAndroidMirrors(t *testing.T) {
	partitions := []disk.PartitionStat{
		{Device: "/dev/block/mmcblk0p1", Mountpoint: "/", Fstype: "erofs"},
		{Device: "/dev/block/mmcblk0p2", Mountpoint: "/data", Fstype: "f2fs"},
		{Device: "/dev/fuse", Mountpoint: "/storage/emulated/0", Fstype: "fuse"},
		{Device: "/dev/fuse", Mountpoint: "/mnt/runtime/default/emulated", Fstype: "fuse"},
	}

	got := selectPhysicalPartitions(partitions)
	if len(got) != 2 {
		t.Fatalf("expected only / and /data, got %+v", got)
	}
	for _, p := range got {
		if p.Mountpoint != "/" && p.Mountpoint != "/data" {
			t.Errorf("unexpected partition kept: %+v", p)
		}
	}
}
