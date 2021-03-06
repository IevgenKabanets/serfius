package api

import (
	"../config"
	"../osinfo"
	serfcli "../serf"
	"fmt"
	"github.com/gin-gonic/gin"
	client "github.com/hashicorp/serf/client"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"
)

var app *gin.Engine

type Register struct {
	NodeName  string `form:"node" json:"node" binding:"required"`
	PrivateIP string `form:"private_ip" json:"private_ip" binding:"required"`
	PublicIP  string `form:"public_ip" json:"public_ip" binding:"required"`
}

type Host struct {
	Node []string `json:"hosts"`
}

type Inventory struct {
	All     Host `json:"all"`
	Engine  Host `json:"docker_engine"`
	Manager Host `json:"docker_swarm_manager"`
	Worker  Host `json:"docker_swarm_worker"`
}

type Msg struct {
	// ClusterID          string
	DockerMasterConfig string
	DockerRole         string
	Hypervisor         string
	Location           string
	MemberAddress      string
	MemberName         string
	MemberPublicIP     string
	NodeExpiration     string
	Status             string
	Team               string
}

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func errorHandle(err error) error {
	if err != nil {
		fmt.Errorf("An error has occured %g", err)
		panic(err)
	}
	return nil
}

func attachRoot(rg *gin.RouterGroup) {

	/**
	 * Global stats
	 */
	rg.GET("/", func(c *gin.Context) {

		c.IndentedJSON(http.StatusOK, gin.H{
			"hostname":       osinfo.Hostname,
			"ipaddr":         osinfo.IPAddress,
			"mem_total":      osinfo.TotalMem,
			"mem_free":       osinfo.FreeMem,
			"mem_user_perc":  osinfo.UsedMem,
			"pid":            os.Getpid(),
			"time":           time.Now(),
			"time_startTime": osinfo.StartTime,
			"time_uptime":    time.Now().Sub(osinfo.StartTime).String()})

	})
}

func attachEndpoints(rg *gin.RouterGroup, cfg config.Config) {

	rg.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	rg.GET("/force_leave/:node", func(c *gin.Context) {
		serf, err := serfcli.NewSerfClient(cfg.Discovery.Server)
		errorHandle(err)
		node := c.Param("node")
		serf.NodeLeave(node)
	})

	// Specifics to Ansible + Swarm provisioning
	rg.GET("/inventory/:team", func(c *gin.Context) {

		serf, err := serfcli.NewSerfClient(cfg.Discovery.Server)
		defer serf.Close()
		errorHandle(err)

		var members *[]client.Member
		allNodes := []string{}
		allManager := []string{}
		allWorker := []string{}
		team := c.Param("team")
		jsons := &Inventory{}

		if team == "all" {
			members, _ = serf.ListAllMembers()
			for _, member := range *members {
				allNodes = append(allNodes, member.Addr.String())
				jsons = &Inventory{
					All: Host{
						Node: allNodes,
					},
				}
			}
		} else {
			status := "alive"
			var tags map[string]string
			tags = make(map[string]string)
			tags["team"] = team

			members, _ = serf.ListMembers(tags, status, "")
			for _, member := range *members {
				allNodes = append(allNodes, member.Addr.String())
				status := osinfo.CheckPort("tcp", member.Addr.String()+":2377")
				match, _ := regexp.MatchString(".*master.*", member.Tags["docker_role"])
				if (status == "Reachable") || match {
					allManager = append(allManager, member.Addr.String())
				} else {
					allWorker = append(allWorker, member.Addr.String())
				}
			}

			jsons = &Inventory{
				All: Host{
					Node: []string{},
				},
				Engine: Host{
					Node: allNodes,
				},
				Manager: Host{
					Node: allManager,
				},
				Worker: Host{
					Node: allWorker,
				},
			}

		}

		c.JSON(http.StatusOK, jsons)
	})

	rg.GET("/member/:name", func(c *gin.Context) {
		serf, err := serfcli.NewSerfClient(cfg.Discovery.Server)
		defer serf.Close()
		errorHandle(err)
		name := c.Param("name")

		var members *[]client.Member
		var msg Msg

		status := "alive"
		var tags map[string]string
		tags = make(map[string]string)
		members, _ = serf.ListMembers(tags, status, name)

		for _, member := range *members {

			msg.DockerMasterConfig = member.Tags["docker_master_config"]
			msg.DockerRole = member.Tags["docker_role"]
			msg.Hypervisor = member.Tags["hypervisor"]
			msg.Location = member.Tags["location"]
			msg.MemberAddress = member.Addr.String()
			msg.MemberName = member.Name
			msg.MemberPublicIP = member.Tags["public_ip"]
			msg.NodeExpiration = member.Tags["node_expiration"]
			msg.Status = member.Status
			msg.Team = member.Tags["team"]

			c.JSON(http.StatusOK, msg)
		}
	})

	rg.GET("/members/:team", func(c *gin.Context) {
		serf, err := serfcli.NewSerfClient(cfg.Discovery.Server)
		defer serf.Close()

		errorHandle(err)
		team := c.Param("team")

		var members *[]client.Member

		if team == "all" {
			members, _ = serf.ListAllMembers()
		} else {
			status := "alive"
			var tags map[string]string
			tags = make(map[string]string)
			tags["team"] = team
			members, _ = serf.ListMembers(tags, status, "")
		}

		var msg Msg
		var res []Msg

		for _, member := range *members {

			msg.DockerMasterConfig = member.Tags["docker_master_config"]
			msg.DockerRole = member.Tags["docker_role"]
			msg.Hypervisor = member.Tags["hypervisor"]
			msg.Location = member.Tags["location"]
			msg.MemberAddress = member.Addr.String()
			msg.MemberName = member.Name
			msg.MemberPublicIP = member.Tags["public_ip"]
			msg.NodeExpiration = member.Tags["node_expiration"]
			msg.Status = member.Status
			msg.Team = member.Tags["team"]
			res = append(res, msg)
		}
		c.JSON(http.StatusOK, res)
	})

	rg.POST("/provision/:env", func(c *gin.Context) {
		leader := c.PostForm("leader")
		team := c.PostForm("team")

		switch env := c.Param("env"); env {
		case "aws":
			c.String(http.StatusOK, "Will provision %s environment with leader %s for team %s", env, leader, team)
		case "xen":
			c.String(http.StatusOK, "Will provision %s environment with leader %s for team %s", env, leader, team)

		}
	})
}

func Start(cfg config.Config) {

	app = gin.New()
	rg := app.Group("/")

	/* attach endpoints */
	attachRoot(rg)
	attachEndpoints(rg, cfg)
	/* run on port */

	if cfg.Api.Bind == "" {
		cfg.Api.Bind = ":4001"
	}

	err := app.Run(cfg.Api.Bind)
	if err != nil {
		log.Fatal(err)
	}

}
