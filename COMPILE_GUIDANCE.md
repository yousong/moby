## Update vendor

* if you need to update dependent package in vendor, you need to add or update package in hack/vendor.sh file, like this:

```
clone git github.com/aliyun-fc/go-loghub 5c958ab5f7b2cd8f9e965fc0724fa3d51941d636
clone git github.com/cloudflare/golz4 ef862a3cdc58a6f1fee4e3af3d44fbe279194cde
clone git github.com/golang/glog 23def4e6c14b4da8ac2ed8007337bc5eb5007998
```

* if your package use 'import "C"'(use cgo) and has necessary .c and .h files, append the directory which contains only .c and .h files to the parameter "findArgs" in hack/.vendor-helpers.sh like this:

```
# lz4 uses cgo and has .c and .h files
findArgs+=( -or -path vendor/src/github.com/cloudflare/golz4/src )
```

* run the command to update vendor

```
bash hack/vendor.sh
```

* run the command to compile binary

```
make bianry
```

* you will see the result in 

```
$ll -rt bundles/*
lrwxrwxrwx  1 yushuting  staff  6 Jun 13 17:46 bundles/latest -> 1.12.6

bundles/1.12.6:
total 0
drwxr-xr-x   6 yushuting  staff  204 Jun 13 17:46 binary-client
drwxr-xr-x  22 yushuting  staff  748 Jun 13 17:48 binary-daemon

$ll -rt *
binary-client:
total 30648
-rw-r--r--  1 yushuting  staff        80 Jun 13 17:46 docker-1.12.6.sha256
-rw-r--r--  1 yushuting  staff        48 Jun 13 17:46 docker-1.12.6.md5
-rwxr-xr-x  1 yushuting  staff  15675704 Jun 13 17:46 docker-1.12.6
lrwxrwxrwx  1 yushuting  staff        13 Jun 13 17:46 docker -> docker-1.12.6

binary-daemon:
total 163160
-rwxr-xr-x  1 yushuting  staff  46168368 Jun 13 17:48 dockerd-1.12.6
-rw-r--r--  1 yushuting  staff        49 Jun 13 17:48 dockerd-1.12.6.md5
lrwxrwxrwx  1 yushuting  staff        14 Jun 13 17:48 dockerd -> dockerd-1.12.6
-rw-r--r--  1 yushuting  staff        81 Jun 13 17:48 dockerd-1.12.6.sha256
-rw-r--r--  1 yushuting  staff        86 Jun 13 17:48 docker-proxy-1.12.6.sha256
-rw-r--r--  1 yushuting  staff        54 Jun 13 17:48 docker-proxy-1.12.6.md5
-rwxr-xr-x  1 yushuting  staff   2879456 Jun 13 17:48 docker-proxy-1.12.6
lrwxrwxrwx  1 yushuting  staff        19 Jun 13 17:48 docker-proxy -> docker-proxy-1.12.6
-rw-r--r--  1 yushuting  staff        52 Jun 13 17:48 docker-containerd.md5
-rwxr-xr-x  1 yushuting  staff  11291144 Jun 13 17:48 docker-containerd
-rw-r--r--  1 yushuting  staff        84 Jun 13 17:48 docker-containerd.sha256
-rw-r--r--  1 yushuting  staff        89 Jun 13 17:48 docker-containerd-shim.sha256
-rw-r--r--  1 yushuting  staff        57 Jun 13 17:48 docker-containerd-shim.md5
-rwxr-xr-x  1 yushuting  staff   3831592 Jun 13 17:48 docker-containerd-shim
-rw-r--r--  1 yushuting  staff        56 Jun 13 17:48 docker-containerd-ctr.md5
-rwxr-xr-x  1 yushuting  staff  10537472 Jun 13 17:48 docker-containerd-ctr
-rw-r--r--  1 yushuting  staff        46 Jun 13 17:48 docker-runc.md5
-rwxr-xr-x  1 yushuting  staff   8764120 Jun 13 17:48 docker-runc
-rw-r--r--  1 yushuting  staff        88 Jun 13 17:48 docker-containerd-ctr.sha256
-rw-r--r--  1 yushuting  staff        78 Jun 13 17:48 docker-runc.sha256
```
