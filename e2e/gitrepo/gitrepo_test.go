package gitrepo_test

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/rancher/fleet/e2e/testenv"
	"github.com/rancher/fleet/e2e/testenv/githelper"
	"github.com/rancher/fleet/e2e/testenv/kubectl"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Git Repo", func() {
	var (
		tmpdir  string
		repodir string
		k       kubectl.Command
		gh      *githelper.Git
		repo    *git.Repository
	)

	replace := func(path string, s string, r string) {
		b, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())

		b = bytes.ReplaceAll(b, []byte(s), []byte(r))

		err = ioutil.WriteFile(path, b, 0644)
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		k = env.Kubectl.Namespace(env.Namespace)
		gh = githelper.New()
		tmpdir, _ = os.MkdirTemp("", "fleet-")

		out, err := k.Create(
			"secret", "generic", "git-auth", "--type", "kubernetes.io/ssh-auth",
			"--from-file=ssh-privatekey="+gh.SSHKey,
			"--from-file=ssh-publickey="+gh.SSHPubKey,
		)
		Expect(err).ToNot(HaveOccurred(), out)

		known := path.Join(tmpdir, "known_hosts")
		os.Setenv("SSH_KNOWN_HOSTS", known)
		out, err = gh.UpdateKnownHosts(known)
		Expect(err).ToNot(HaveOccurred(), out)

		repodir = path.Join(tmpdir, "repo")
		repo, err = gh.Create(repodir, testenv.AssetPath("gitrepo/sleeper-chart"), "examples")
		Expect(err).ToNot(HaveOccurred())

		tmpl := template.Must(template.New("test").Parse(githelper.GitRepoTemplate))
		var yaml strings.Builder
		err = tmpl.Execute(&yaml, githelper.GitRepo{Repo: gh.URL})
		Expect(err).ToNot(HaveOccurred())
		fleet := path.Join(tmpdir, "fleet.yaml")
		os.WriteFile(fleet, []byte(yaml.String()), 0644)

		out, err = k.Apply("-f", fleet)
		Expect(err).ToNot(HaveOccurred(), out)
	})

	AfterEach(func() {
		os.RemoveAll(tmpdir)
		k.Delete("secret", "git-auth")
		k.Delete("gitrepo", "testing")
	})

	When("updating a git repository", func() {
		It("updates the deployment", func() {
			By("checking the pod exists")
			Eventually(func() string {
				out, _ := k.Namespace("default").Get("pods")
				return out
			}, testenv.Timeout).Should(ContainSubstring("sleeper-"))

			By("updating the git repository")
			replace(path.Join(repodir, "examples", "Chart.yaml"), "0.1.0", "0.2.0")
			replace(path.Join(repodir, "examples", "templates", "deployment.yaml"), "name: sleeper", "name: newsleep")

			commit, err := gh.Update(repo)
			Expect(err).ToNot(HaveOccurred())

			By("checking for the updated commit hash in gitrepo")
			Eventually(func() string {
				out, _ := k.Get("gitrepo", "testing", "-o", "yaml")
				return out
			}, testenv.Timeout).Should(ContainSubstring("commit: " + commit))

			By("checking the deployment's new name")
			Eventually(func() string {
				out, _ := k.Namespace("default").Get("deployments")
				return out
			}, testenv.Timeout).Should(ContainSubstring("newsleep"))

		})
	})
})
