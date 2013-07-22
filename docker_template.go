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
	` && echo {{.Id}} >> /etc/action/box_id` +
	` {{ range $i, $key := .PublicKeys }} echo "{{$key}}" >> /etc/ssh/univ_public_keys {{end}}` +
	`
	# THE END
	`

var DockerfileCompiled *template.Template

func init() {
	var err error
	DockerfileCompiled, err = template.New("dockerfile").Parse(DockerfileTemplate)
	if err != nil {
		panic(err)
	}
}
