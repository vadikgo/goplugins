# goplugins

```text
Usage of ./goplugins:
  -dest string
      YAML with updated plugins versions (default "jenkins_plugins_latest.yml")
  -jenkins string
      Jenkins version for check compatibility (default "2.222.2")
  -src string
      Source file with jenkins_plugins (default "jenkins_plugins_test.yml")
  -version
      Print version information.
```

## build with version info

```bash
go build -ldflags="-X \"main.buildstamp=$(date)\" -X \"main.githash=$(git rev-parse HEAD)\"" goplugins.go
```

## build linux binary in docker

```bash
docker run -it --name centos7 -v $(pwd):/devel centos:7 bash
yum install epel-release -y
yum install golang zip -y
cd /devel
egrep -o '"github.com.+' goplugins.go | xargs -I {} go get {}
go build -ldflags="-X \"main.buildstamp=$(date)\" -X \"main.githash=$(git rev-parse HEAD)\"" goplugins.go
zip goplugins-linux-amd64.zip goplugins
```
