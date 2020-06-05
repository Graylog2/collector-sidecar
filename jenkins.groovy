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
             stash name: 'build artifacts', includes: 'build/**'
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
            unstash 'build artifacts'
            sh 'make package-all'
            stash name: 'package artifacts', includes: 'dist/pkg/**'
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
           unstash 'package artifacts'

           script
           {
             def RELEASE_DATA = sh returnStdout: true, script: "curl -s --user \"$GITHUB_CREDS\" -X POST --data \'{ \"tag_name\": \"${TAG_NAME}\", \"name\": \"${TAG_NAME}\", \"body\": \"Insert features here.\"}\' https://api.github.com/repos/Graylog2/collector-sidecar/releases"
             def props = readJSON text: RELEASE_DATA
             echo RELEASE_DATA
             if (props.id)
             {
               env.RELEASE_ID = props.id
             }
             else
             {
               error('Github Release ID is null.')
             }

             sh '''#!/bin/bash
                 set -x
                 for file in dist/pkg/*
                 do
                   FILENAME=$(basename $file)
                   curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/$FILENAME https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=$FILENAME
                 done
             '''
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
