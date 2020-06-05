pipeline
{
   agent none

   options
   {
      buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '100', daysToKeepStr: '30', numToKeepStr: '100')
      timestamps()
   }

   tools
   {
     go 'Go'
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
            label 'linux'
          }
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
             label 'linux'
             image 'torch/jenkins-fpm-cook:latest'
             args '-u jenkins:docker'
           }
         }

         steps
         {
            sh 'make package-all'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
            }
         }
      }

      stage('Release')
      {
         when
         {
             buildingTag()
         }

         agent
         {
           label 'linux'
         }

         environment
         {
             GITHUB_CREDS = credentials('github-access-token')
         }

         steps
         {
           echo "Releasing ${TAG_NAME} to Github..."

           script
           {
             def RELEASE_DATA = sh returnStdout: true, script: "curl -s --user \"$GITHUB_CREDS\" -X POST --data \'{ \"tag_name\": \"${TAG_NAME}\", \"name\": \"${TAG_NAME}\", \"body\": \"Insert features here.\", \"draft\": true }\' https://api.github.com/repos/Graylog2/collector-sidecar/releases"
             def props = readJSON text: RELEASE_DATA
             env.RELEASE_ID = props.id

             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar-1.1.0-SNAPSHOT.tar.gz https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar-1.1.0-SNAPSHOT.tar.gz'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar-1.1.0-0.SNAPSHOT.armv7.rpm https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar-1.1.0-0.SNAPSHOT.armv7.rpm'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar-1.1.0-0.SNAPSHOT.i386.rpm https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar-1.1.0-0.SNAPSHOT.i386.rpm'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar-1.1.0-0.SNAPSHOT.x86_64.rpm https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar-1.1.0-0.SNAPSHOT.x86_64.rpm'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar_1.1.0-0.SNAPSHOT_amd64.deb https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar_1.1.0-0.SNAPSHOT_amd64.deb'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar_1.1.0-0.SNAPSHOT_armv7.deb https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar_1.1.0-0.SNAPSHOT_armv7.deb'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/graylog-sidecar_1.1.0-0.SNAPSHOT_i386.deb https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-sidecar_1.1.0-0.SNAPSHOT_i386.deb'
           }
         }
         post
         {
           success
           {
             script
             {
                cleanWs()
             }
           }
         }
      }
   }
}
