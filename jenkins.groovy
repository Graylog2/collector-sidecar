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
    go 'Go 1.21'
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

        // Sign the Windows binaries before we build the installer .exe to
        // ensure that the signed graylog-sidecar.exe binaries are included
        // in the installer.
        stage('Sign Windows Binaries')
        {
          agent
          {
            docker
            {
              image 'graylog/internal-codesigntool:latest'
              args '-u jenkins:jenkins'
              registryCredentialsId 'docker-hub'
              alwaysPull true
              reuseNode true
            }
          }

          environment
          {
            CODESIGN_USER = credentials('codesign-user')
            CODESIGN_PASS = credentials('codesign-pass')
            CODESIGN_TOTP_SECRET = credentials('codesign-totp-secret')
            CODESIGN_CREDENTIAL_ID = credentials('codesign-credential-id')
          }

          steps
          {
            sh 'make sign-binaries'
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

        stage('Sign Windows Installer')
        {
          agent
          {
            docker
            {
              image 'graylog/internal-codesigntool:latest'
              args '-u jenkins:jenkins'
              registryCredentialsId 'docker-hub'
              alwaysPull false // We did that in the previous sign stage
              reuseNode true
            }
          }

          environment
          {
            CODESIGN_USER = credentials('codesign-user')
            CODESIGN_PASS = credentials('codesign-pass')
            CODESIGN_TOTP_SECRET = credentials('codesign-totp-secret')
            CODESIGN_CREDENTIAL_ID = credentials('codesign-credential-id')
          }

          steps
          {
            sh 'make sign-windows-installer'
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

        stage('Create Checksums')
        {
          steps
          {
            dir('dist/pkg')
            {
              sh 'sha256sum * | tee CHECKSUMS-SHA256.txt'
            }
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

        stage('GitHub Release')
        {
          when
          {
            buildingTag()
          }

          environment
          {
            GITHUB_CREDS = credentials('github-access-token')
            REPO_API_URL = 'https://api.github.com/repos/Graylog2/collector-sidecar'
            UPLOAD_API_URL = 'https://uploads.github.com/repos/Graylog2/collector-sidecar'
          }

          steps
          {
            echo "Releasing ${env.TAG_NAME} to GitHub..."

            script
            {
              def RELEASE_DATA = sh returnStdout: true, script: "curl -fsSL --user \"$GITHUB_CREDS\" --data \'{ \"tag_name\": \"${TAG_NAME}\", \"name\": \"${TAG_NAME}\", \"body\": \"Insert changes here.\", \"draft\": true }\' $REPO_API_URL/releases"
              def props = readJSON text: RELEASE_DATA
              env.RELEASE_ID = props.id

              sh '''#!/bin/bash
                set -xeo pipefail

                for file in dist/pkg/*; do
                  name="$(basename "$file")"

                  curl -fsSL \
                    -H "Authorization: token $GITHUB_CREDS" \
                    -H "Content-Type: application/octet-stream" \
                    --data-binary "@$file" \
                    "$UPLOAD_API_URL/releases/$RELEASE_ID/assets?name=$name"
                done
              '''
            }
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
