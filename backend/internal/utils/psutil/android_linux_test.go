//go:build linux

package psutil

import (
	"reflect"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestParseMountinfo(t *testing.T) {
	data := []byte(`36 35 259:2 / / rw,relatime - f2fs /dev/root rw
37 35 259:3 / /data rw,relatime - f2fs /dev/block/mmcblk0p3 rw
40 36 0:40 / /dev/shm rw,nosuid,nodev - tmpfs tmpfs rw
41 37 /data/media/0 /storage/emulated/0 rw,nosuid,nodev,relatime - fuse /dev/fuse rw`)

	got, err := parseMountinfo(data, "/proc/1/root")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d partitions, want 4: %+v", len(got), got)
	}
	if got[0].Mountpoint != "/" || got[0].Fstype != "f2fs" {
		t.Errorf("unexpected root partition: %+v", got[0])
	}
	if got[1].Mountpoint != "/data" || got[1].Fstype != "f2fs" {
		t.Errorf("unexpected /data partition: %+v", got[1])
	}
	if got[2].Mountpoint != "/dev/shm" || got[2].Fstype != "tmpfs" {
		t.Errorf("unexpected tmpfs partition: %+v", got[2])
	}
	if got[3].Mountpoint != "/storage/emulated/0" || got[3].Fstype != "fuse" {
		t.Errorf("unexpected fuse partition: %+v", got[3])
	}
}

func TestParseMounts(t *testing.T) {
	data := []byte("/dev/root / ext4 rw,relatime 0 0\n" +
		"tmpfs /dev/shm tmpfs rw,nosuid,nodev 0 0\n")
	got, err := parseMounts(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []disk.PartitionStat{
		{Device: "/dev/root", Mountpoint: "/", Fstype: "ext4", Opts: []string{"rw", "relatime", "0", "0"}},
		{Device: "tmpfs", Mountpoint: "/dev/shm", Fstype: "tmpfs", Opts: []string{"rw", "nosuid", "nodev", "0", "0"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseMounts = %+v, want %+v", got, want)
	}
}

func TestPidFromHostRoot(t *testing.T) {
	cases := map[string]int{
		"/proc/657/root":  657,
		"/proc/1/root":    1,
		"/proc/657/root/": 657,
		"":                0,
		"/data/local":     0,
		"/proc/abc/root":  0,
	}
	for root, want := range cases {
		if got := pidFromHostRoot(root); got != want {
			t.Errorf("pidFromHostRoot(%q) = %d, want %d", root, got, want)
		}
	}
}
