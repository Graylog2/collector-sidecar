pipeline
{
   agent
   {
     label 'test'
     customWorkspace '/home/jenkins/go/src/github.com/Graylog2/collector-sidecar'
   }

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
     GO15VENDOREXPERIMENT=1
   }

   stages
   {
      stage('Install Deps')
      {
         steps
         {
            sh 'go version'
            sh 'go env'
            sh 'glide update'
            sh 'glide install'
         }
     }
      stage('Test')
      {
          steps
          {
             sh "make test"
          }
       }
      stage('Build')
      {
         steps
         {
            sh 'make build-all'
         }
      }

      stage('Package')
      {
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

         environment
         {
             GITHUB_CREDS = credentials('github-access-token')
         }

         steps
         {
           echo "Releasing ${TAG_NAME} to Github..."

           script
           {
             def RELEASE_DATA = sh returnStdout: true, script: "curl -s --user \"$GITHUB_CREDS\" -X POST --data \'{ \"tag_name\": \"${TAG_NAME}\", \"name\": \"v${TAG_NAME}\", \"body\": \"Insert features here.\", \"draft\": true }\' https://api.github.com/repos/Graylog2/collector-sidecar/releases"
             def props = readJSON text: RELEASE_DATA
             env.RELEASE_ID = props.id

             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @graylog-project.linux https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=graylog-project.linux'
             sh 'curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @graylog-project.linux https://uploads.github.com/repos/Graylog2/graylog-project-cli/releases/$RELEASE_ID/assets?name=graylog-project.darwin'
           }
         }
      }
   }
}
