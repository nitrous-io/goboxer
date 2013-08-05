package goboxer

import (
	"text/template"
)

const DockerfileTemplate = `
FROM  {{.BaseImage}}
RUN` +
	` mkdir -p /home/action/.ssh` +
	` && ssh-keygen -N '' -f /home/action/.ssh/id_rsa -b 1024 -t rsa -q` +
	` && usermod -u {{.Uid}} action` +
	` && groupmod -g {{.Gid}} action` +
	` && chown -R {{.Uid}}:{{.Uid}} /home/action/.[^.]*` +
	` && echo {{.Id}} >> /etc/action/box_id` +
	` {{ range $i, $key := .PublicKeys }} && echo "{{$key}}" >> /etc/ssh/univ_public_keys {{end}}` +
	`
	# THE END
	`

const MigrateDockerfileTemplate = `
FROM {{.Image}}
RUN` +
	` usermod -u {{.Uid}} action` +
	` && groupmod -g {{.Gid}} action` +
	` && chown -R {{.Uid}}:{{.Gid}} /home/action` +
	` && echo {{.Id}} >> /etc/action/box_id` +
	` {{ range $i, $key := .PublicKeys }} && echo "{{$key}}" >> /etc/ssh/univ_public_keys {{end}}` +
	`
	# THE END
	`

var DockerfileCompiled *template.Template
var MigrateDockerfileCompiled *template.Template

func init() {
	var err error
	DockerfileCompiled, err = template.New("dockerfile").Parse(DockerfileTemplate)
	if err != nil {
		panic(err)
	}
	MigrateDockerfileCompiled, err =
		template.New("migrate_dockerfile").Parse(MigrateDockerfileTemplate)
	if err != nil {
		panic(err)
	}
}
