if (currentBuild.buildCauses.toString().contains('BranchIndexingCause'))
{
  print "Build skipped due to trigger being Branch Indexing."
  return
}

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
            stash name: 'package artifacts', includes: 'dist/**'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
            }
         }
      }

      stage('Upload packages to S3')
      {
         when
         {
             buildingTag()
         }

         agent
         {
           label 'linux'
         }

         steps
         {
           unstash 'package artifacts'
           echo "Uploading to S3..."
           s3Upload(workingDir:'dist/pkg', bucket:'graylog2-releases', path:"graylog-collector-sidecar/${TAG_NAME}", includePathPattern:'graylog*')
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

      // stage('Release to Package Repository')
      // {
      //    when
      //    {
      //        buildingTag()
      //    }
      //
      //    agent
      //    {
      //      label 'packages'
      //    }
      //
      //    steps
      //    {
      //      echo "Checking out fpm-recipes..."
      //      checkout poll: false, scm: [$class: 'GitSCM', branches: [[name: '*/4.0']], doGenerateSubmoduleConfigurations: false, extensions: [[$class: 'WipeWorkspace']], submoduleCfg: [], userRemoteConfigs: [[credentialsId: 'github-access-token2', url: 'https://github.com/Graylog2/fpm-recipes.git']]]
      //
      //      script
      //      {
      //        sh "gl2-build-pkg-sidecar 1.1"
      //      }
      //    }
      //    post
      //    {
      //      success
      //      {
      //        script
      //        {
      //           cleanWs()
      //        }
      //      }
      //    }
      // }

      stage('Release to Github')
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

           catchError(buildResult: 'SUCCESS', stageResult: 'FAILURE')
           {
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

               sh "docker run --rm -v $WORKSPACE:$WORKSPACE -w $WORKSPACE/dist/chocolatey torch/jenkins-mono-choco:latest pack --version ${TAG_NAME}"
             }
           }
         }
         post
         {
           success
           {
             script
             {
                archiveArtifacts 'dist/chocolatey/*.nupkg'
                cleanWs()
             }
           }
         }
      }
   }
}
