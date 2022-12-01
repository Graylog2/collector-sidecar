pipeline
{
  agent none

  options
  {
    buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '10', daysToKeepStr: '30', numToKeepStr: '10')
    timestamps()
    withAWS(region:'eu-west-1', credentials:'aws-key-releases')
  }

  tools
  {
    go 'Go 1.19'
  }

  environment
  {
    GOPATH = '/home/jenkins/go'
    GO15VENDOREXPERIMENT=1
  }

  stages
  {
    stage('Build')
    {
      agent
      {
        // Select the node for all nested stages.
        label 'linux'
      }

      // All nested stages run on the same node because the nested stages
      // don't have an agent label selector.
      // That ensures we share the workspace between the different stages.
      stages
      {
        stage('Compile')
        {
          steps
          {
            sh 'go version'
            sh 'go mod vendor'
            sh "make test"
            sh 'make build-all'
          }
        }

        stage('Package')
        {
          agent
          {
            docker
            {
              image 'torch/jenkins-fpm-cook:latest'
              args '-u jenkins:docker'
              reuseNode true
            }
          }

          steps
          {
            sh 'make package-all'
          }
        }

        stage('Chocolatey Pack')
        {
          agent
          {
            dockerfile
            {
              label 'linux'
              filename 'Dockerfile.chocolatey'
              dir 'docker'
              additionalBuildArgs '-t local/sidecar-chocolatey'
              reuseNode true
            }
          }

          steps
          {
            sh 'make package-chocolatey'
          }
        }

        stage('Chocolatey Push')
        {
          when
          {
            buildingTag()
          }

          agent
          {
            dockerfile
            {
              label 'linux'
              filename 'Dockerfile.chocolatey'
              dir 'docker'
              additionalBuildArgs '-t local/sidecar-chocolatey'
              reuseNode true
            }
          }

          environment
          {
            CHOCO_API_KEY = credentials('chocolatey-api-key')
          }

          steps
          {
            sh 'make push-chocolatey'
          }
        }

        stage('Upload')
        {
          when
          {
            buildingTag()
          }

          steps
          {
            echo "==> Artifact checksums:"
            sh "sha256sum dist/pkg/*"

            s3Upload(
              workingDir: '.',
              bucket: 'graylog2-releases',
              path: "graylog-collector-sidecar/${env.TAG_NAME}/",
              file: "dist/pkg"
            )
          }
        }
      }

      post
      {
        success
        {
          archiveArtifacts 'dist/pkg/*'
        }

        cleanup
        {
          cleanWs()
        }
      }
    }
  }
}
